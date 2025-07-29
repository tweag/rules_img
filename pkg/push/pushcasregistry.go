package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/bazelbuild/rules_go/go/runfiles"
	"github.com/malt3/go-containerregistry/pkg/authn"
	"github.com/malt3/go-containerregistry/pkg/name"
	registryv1 "github.com/malt3/go-containerregistry/pkg/v1"
	"github.com/malt3/go-containerregistry/pkg/v1/remote"
	typesv1 "github.com/malt3/go-containerregistry/pkg/v1/types"

	"github.com/tweag/rules_img/pkg/api"
	blobcache_proto "github.com/tweag/rules_img/pkg/proto/blobcache"
	remoteexecution_proto "github.com/tweag/rules_img/pkg/proto/remote-apis/build/bazel/remote/execution/v2"
	registrytypes "github.com/tweag/rules_img/pkg/serve/registry/types"
)

type casRegistryPusher struct {
	blobcacheClient blobcache_proto.BlobsClient
}

func NewCASRegistryPusher(blobcacheClient blobcache_proto.BlobsClient) *casRegistryPusher {
	return &casRegistryPusher{
		blobcacheClient: blobcacheClient,
	}
}

func (p casRegistryPusher) Push(ctx context.Context, reference string, req registrytypes.PushRequest) (string, error) {
	if len(req.Blobs) == 0 {
		return "", errors.New("no blobs to push")
	}

	digestFunction := remoteexecution_proto.DigestFunction_SHA256
	switch req.Blobs[0].Digest[:6] {
	case "sha256":
		digestFunction = remoteexecution_proto.DigestFunction_SHA256
	case "sha512":
		digestFunction = remoteexecution_proto.DigestFunction_SHA512
	}
	var blobDigests []*remoteexecution_proto.Digest
	for _, blob := range req.Blobs {
		blobDigests = append(blobDigests, &remoteexecution_proto.Digest{
			Hash:      blob.Digest[7:], // Skip the "sha256:" prefix.
			SizeBytes: blob.Size,
		})
	}
	resp, err := p.blobcacheClient.Commit(ctx, &blobcache_proto.CommitRequest{
		BlobDigests:    blobDigests,
		DigestFunction: digestFunction,
	})
	if err != nil {
		return "", fmt.Errorf("committing blobs to CAS registry: %w", err)
	}
	knownMissing := map[string]struct{}{}
	for _, hash := range req.MissingBlobs {
		knownMissing[hash] = struct{}{}
	}
	for _, digest := range resp.MissingBlobDigests {
		if _, ok := knownMissing[digest.Hash]; !ok {
			return "", fmt.Errorf("missing blob %s in CAS registry that should be present", digest.Hash)
		}
	}

	updateChan := make(chan registryv1.Update, 64)
	go progressPrinter(updateChan)

	transport := remote.DefaultTransport.(*http.Transport).Clone()
	// Ironically, the go-containerregistry HTTP transport forces HTTP/2,
	// while the go-containerregistry registry HTTP server doesn't handle HTTP/2 well.
	// This is a workaround to force HTTP/1.1 for the push operation.
	transport.ForceAttemptHTTP2 = false
	// The registry has a bug that causes long moments of inactivity.
	// See also: https://github.com/google/go-containerregistry/issues/2120
	transport.IdleConnTimeout = 30 * time.Minute
	transport.DialContext = (&net.Dialer{
		Timeout:   1800 * time.Second,
		KeepAlive: 10 * time.Second,
	}).DialContext
	transport.ExpectContinueTimeout = 30 * time.Minute
	transport.MaxIdleConns = 0

	opts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithProgress(updateChan),
		remote.WithTransport(transport),
	}
	ref, err := name.ParseReference(reference)
	if err != nil {
		return "", err
	}

	mediaType := typesv1.MediaType(req.Blobs[0].MediaType)
	if mediaType.IsImage() {
		manifest, err := p.pushableImage(ctx, req, "manifest.json", knownMissing)
		if err != nil {
			return "", err
		}
		if err := remote.Write(ref, manifest, opts...); err != nil {
			return "", err
		}
	} else if mediaType.IsIndex() {
		index, err := p.pushableIndex(ctx, req, req.Blobs[0], knownMissing)
		if err != nil {
			return "", err
		}
		if err := remote.WriteIndex(ref, index, opts...); err != nil {
			return "", err
		}
	} else {
		return "", fmt.Errorf("unsupported media type %s for push", mediaType)
	}

	return req.Blobs[0].Digest, nil
}

func (p casRegistryPusher) pushableImage(ctx context.Context, req registrytypes.PushRequest, manifestBasePath string, knownMissing map[string]struct{}) (*pushableImage, error) {
	manifestPath := manifestBasePath
	if manifestBasePath != "manifest.json" {
		manifestPath = filepath.Join(manifestBasePath, "manifest.json")
	}
	configPath := "config.json"
	if manifestBasePath != "manifest.json" {
		configPath = filepath.Join(manifestBasePath, "config.json")
	}
	rawManifest, err := readFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest registryv1.Manifest
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		return nil, err
	}

	rawConfig, err := readFile(configPath)
	if err != nil {
		return nil, err
	}
	var config registryv1.ConfigFile
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, err
	}
	layers := make([]registryv1.Layer, len(manifest.Layers))
	for i, layer := range manifest.Layers {
		var remoteBlob compressedReader
		if _, ok := knownMissing[layer.Digest.Hex]; !ok {
			// Since we are using a special container registry that is directly connected to the CAS,
			// we shouldn't need to upload any blobs.
			// This is a stub implementation that fails if any layer is requested.
			remoteBlob = &stubBlob{}
		} else {
			// If the layer is known to be missing, we don't need to create a remote blob for it.
			remoteBlob = newRemoteBlob(toAPIDescriptor(layer), req.PullInfo)
		}
		layers[i] = &pushableLayer{
			metadata: toAPIDescriptor(layer),
			remote:   remoteBlob,
		}
	}

	return &pushableImage{
		rawManifest: rawManifest,
		rawConfig:   rawConfig,
		manifest:    &manifest,
		config:      &config,
		layers:      layers,
	}, nil
}

func (p casRegistryPusher) pushableIndex(ctx context.Context, req registrytypes.PushRequest, descriptor api.Descriptor, knownMissing map[string]struct{}) (*pushableIndex, error) {
	rawIndex, err := readFile("index.json")
	if err != nil {
		return nil, err
	}
	var index registryv1.IndexManifest
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		return nil, fmt.Errorf("parsing image index: %w", err)
	}

	manifests := make([]*pushableImage, len(index.Manifests))
	for i := range index.Manifests {
		manifest, err := p.pushableImage(ctx, req, filepath.Join("manifest", strconv.Itoa(i)), knownMissing)
		if err != nil {
			return nil, err
		}
		manifests[i] = manifest
	}

	return &pushableIndex{
		rawIndex:  rawIndex,
		index:     &index,
		manifests: manifests,
	}, nil
}

func readFile(path string) ([]byte, error) {
	path, err := runfiles.Rlocation(path)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", path, err)
	}
	return os.ReadFile(path)
}
