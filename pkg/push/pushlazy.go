package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bazelbuild/rules_go/go/runfiles"
	"github.com/malt3/go-containerregistry/pkg/authn"
	"github.com/malt3/go-containerregistry/pkg/name"
	registryv1 "github.com/malt3/go-containerregistry/pkg/v1"
	"github.com/malt3/go-containerregistry/pkg/v1/remote"
	typesv1 "github.com/malt3/go-containerregistry/pkg/v1/types"

	"github.com/tweag/rules_img/pkg/api"
	"github.com/tweag/rules_img/pkg/cas"
	registrytypes "github.com/tweag/rules_img/pkg/serve/registry/types"
)

type lazyPusher struct {
	casReader        blobReader
	checkConsistency bool
}

func NewLazy(casReader blobReader) *lazyPusher {
	return &lazyPusher{
		casReader: casReader,
	}
}

func (p lazyPusher) Push(ctx context.Context, reference string, req registrytypes.PushRequest, options ...lazyOpt) (string, error) {
	var opt lazyOpts
	for _, o := range options {
		o(&opt)
	}

	if len(req.Blobs) == 0 {
		return "", errors.New("no blobs to push")
	}

	knownMissing := make(map[string]struct{}, len(req.MissingBlobs))
	for _, hash := range req.MissingBlobs {
		knownMissing[hash] = struct{}{}
	}
	var expectPresent []cas.Digest
	for _, blob := range req.Blobs {
		if !strings.HasPrefix(blob.Digest, "sha256:") {
			return "", fmt.Errorf("invalid blob digest %s: expected sha256 prefix", blob.Digest)
		}
		if _, ok := knownMissing[blob.Digest[7:]]; ok {
			continue // Skip blobs that are known to be missing.
		}
		digest, err := digestFromDescriptor(blob)
		if err != nil {
			return "", fmt.Errorf("invalid blob digest %s: %w", blob.Digest, err)
		}
		expectPresent = append(expectPresent, digest)
	}
	if opt.checkConsistency {
		missing, err := p.casReader.FindMissingBlobs(ctx, expectPresent)
		if err != nil {
			return "", fmt.Errorf("checking for missing blobs: %w", err)
		}
		var missingHashes []string
		for _, digest := range missing {
			missingHashes = append(missingHashes, fmt.Sprintf("%x", digest.Hash))
		}
		if len(missing) > 0 {
			return "", fmt.Errorf("missing blobs: %s", strings.Join(missingHashes, ", "))
		}
	}

	updateChan := make(chan registryv1.Update, 64)
	go progressPrinter(updateChan)
	opts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithProgress(updateChan),
	}
	ref, err := name.ParseReference(reference)
	if err != nil {
		return "", err
	}

	mediaType := typesv1.MediaType(req.Blobs[0].MediaType)
	if mediaType.IsImage() {
		manifest, err := p.pushableImage(ctx, req, req.Blobs[0], "index.json", knownMissing)
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

func (p lazyPusher) pushableImage(ctx context.Context, req registrytypes.PushRequest, descriptor api.Descriptor, manifestBasePath string, knownMissing map[string]struct{}) (*pushableImage, error) {
	manifestPath := manifestBasePath
	if manifestBasePath != "index.json" {
		manifestPath = filepath.Join(manifestBasePath, "manifest.json")
	}
	configPath := "config.json"
	if manifestBasePath != "index.json" {
		configPath = filepath.Join(manifestBasePath, "config.json")
	}
	rawManifest, err := readFileOrCAS(ctx, p.casReader, manifestPath, descriptor)
	if err != nil {
		return nil, err
	}
	var manifest registryv1.Manifest
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		return nil, err
	}

	configDescriptor := toAPIDescriptor(manifest.Config)
	rawConfig, err := readFileOrCAS(ctx, p.casReader, configPath, configDescriptor)
	if err != nil {
		return nil, err
	}
	var config registryv1.ConfigFile
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, err
	}
	layers := make([]registryv1.Layer, len(manifest.Layers))
	for i, layer := range manifest.Layers {
		if _, ok := knownMissing[layer.Digest.String()]; ok {
			// Layer is from base image
			layers[i] = &pushableLayer{
				metadata: toAPIDescriptor(layer),
				remote:   newRemoteBlob(toAPIDescriptor(layer), req.PullInfo),
			}
		} else {
			// Layer can be served from the CAS.
			layers[i] = newMetadataLayer(toAPIDescriptor(layer), p.casReader)
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

func (p lazyPusher) pushableIndex(ctx context.Context, req registrytypes.PushRequest, descriptor api.Descriptor, knownMissing map[string]struct{}) (*pushableIndex, error) {
	rawIndex, err := readFileOrCAS(ctx, p.casReader, "index.json", descriptor)
	if err != nil {
		return nil, err
	}
	var index registryv1.IndexManifest
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		return nil, fmt.Errorf("parsing image index: %w", err)
	}

	manifests := make([]*pushableImage, len(index.Manifests))
	for i, manifestDesc := range index.Manifests {
		manifest, err := p.pushableImage(ctx, req, toAPIDescriptor(manifestDesc), filepath.Join("manifest", strconv.Itoa(i)), knownMissing)
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

func toAPIDescriptor(d registryv1.Descriptor) api.Descriptor {
	return api.Descriptor{
		MediaType: string(d.MediaType),
		Digest:    d.Digest.String(),
		Size:      d.Size,
	}
}

func readFileOrCAS(ctx context.Context, casReader blobReader, filePath string, descriptor api.Descriptor) ([]byte, error) {
	maybeContents, err := func() ([]byte, error) {
		filePath, err := runfiles.Rlocation(filePath)
		if err != nil {
			return nil, nil
		}
		rawFile, err := os.ReadFile(filePath)
		if err == nil {
			return rawFile, nil
		}
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}()
	if err != nil {
		return nil, err
	}
	if maybeContents != nil {
		return maybeContents, nil
	}

	digest, err := digestFromDescriptor(descriptor)
	if err != nil {
		return nil, err
	}
	// If the file does not exist, try to read it from the CAS.
	return casReader.ReadBlob(ctx, digest)
}

type lazyOpts struct {
	checkConsistency bool
}

type lazyOpt func(*lazyOpts)

func WithConsistencyCheck() lazyOpt {
	return func(opts *lazyOpts) {
		opts.checkConsistency = true
	}
}
