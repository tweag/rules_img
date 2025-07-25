package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/bazelbuild/rules_go/go/runfiles"
	"github.com/malt3/go-containerregistry/pkg/authn"
	"github.com/malt3/go-containerregistry/pkg/name"
	registryv1 "github.com/malt3/go-containerregistry/pkg/v1"
	"github.com/malt3/go-containerregistry/pkg/v1/remote"
	typesv1 "github.com/malt3/go-containerregistry/pkg/v1/types"

	"github.com/tweag/rules_img/pkg/api"
	registrytypes "github.com/tweag/rules_img/pkg/serve/registry/types"
)

type lazyPusher struct {
	casReader blobReader
}

func NewLazy(casReader blobReader) *lazyPusher {
	return &lazyPusher{
		casReader: casReader,
	}
}

func (p lazyPusher) Push(ctx context.Context, reference string, req registrytypes.PushRequest) (string, error) {
	if len(req.Blobs) == 0 {
		return "", errors.New("no blobs to push")
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
		manifest, err := p.pushableImage(ctx, req, req.Blobs[0], "index.json")
		if err != nil {
			return "", err
		}
		if err := remote.Write(ref, manifest, opts...); err != nil {
			return "", err
		}
	} else if mediaType.IsIndex() {
		index, err := p.pushableIndex(ctx, req, req.Blobs[0])
		if err != nil {
			return "", err
		}
		if err := remote.WriteIndex(ref, index, opts...); err != nil {
			return "", err
		}
	}

	return req.Blobs[0].Digest, nil
}

func (p lazyPusher) pushableImage(ctx context.Context, req registrytypes.PushRequest, descriptor api.Descriptor, manifestBasePath string) (*pushableImage, error) {
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
		layers[i] = newMetadataLayer(toAPIDescriptor(layer), p.casReader)
	}

	return &pushableImage{
		rawManifest: rawManifest,
		rawConfig:   rawConfig,
		manifest:    &manifest,
		config:      &config,
		layers:      layers,
	}, nil
}

func (p lazyPusher) pushableIndex(ctx context.Context, req registrytypes.PushRequest, descriptor api.Descriptor) (*pushableIndex, error) {
	rawIndex, err := readFileOrCAS(ctx, p.casReader, "index.json", descriptor)
	if err != nil {
		return nil, err
	}
	var index registryv1.IndexManifest
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		return nil, fmt.Errorf("parsing imgge index: %w", err)
	}

	manifests := make([]*pushableImage, len(index.Manifests))
	for i, manifestDesc := range index.Manifests {
		manifest, err := p.pushableImage(ctx, req, toAPIDescriptor(manifestDesc), filepath.Join("manifest", strconv.Itoa(i)))
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
