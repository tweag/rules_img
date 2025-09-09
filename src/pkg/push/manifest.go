package push

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"

	"github.com/bazelbuild/rules_go/go/runfiles"
	registryv1 "github.com/malt3/go-containerregistry/pkg/v1"
	types "github.com/malt3/go-containerregistry/pkg/v1/types"
)

type pushableImage struct {
	rawManifest []byte
	rawConfig   []byte
	manifest    *registryv1.Manifest
	config      *registryv1.ConfigFile
	layers      []registryv1.Layer
}

func newPushableImage(req PushManifestRequest) (*pushableImage, error) {
	manifestPath, err := runfiles.Rlocation(req.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("reading manifest file: %w", err)
	}
	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	configPath, err := runfiles.Rlocation(req.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	rawConfig, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var manifest registryv1.Manifest
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		return nil, err
	}
	var config registryv1.ConfigFile
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, err
	}
	layers := make([]registryv1.Layer, len(req.Layers))
	for i, layerInput := range req.Layers {
		layer, err := newPushableLayer(layerInput, req.RemoteBlobInfo)
		if err != nil {
			return nil, err
		}
		layers[i] = layer
	}
	return &pushableImage{
		rawManifest: rawManifest,
		rawConfig:   rawConfig,
		manifest:    &manifest,
		config:      &config,
		layers:      layers,
	}, nil
}

// Layers returns the ordered collection of filesystem layers that comprise this image.
// The order of the list is oldest/base layer first, and most-recent/top layer last.
func (i *pushableImage) Layers() ([]registryv1.Layer, error) {
	return i.layers, nil
}

// MediaType of this image's manifest.
func (i *pushableImage) MediaType() (types.MediaType, error) {
	return i.manifest.MediaType, nil
}

// Size returns the size of the manifest.
func (i *pushableImage) Size() (int64, error) {
	return int64(len(i.rawManifest)), nil
}

// ConfigName returns the hash of the image's config file, also known as
// the Image ID.
func (i *pushableImage) ConfigName() (registryv1.Hash, error) {
	return i.manifest.Config.Digest, nil
}

// ConfigFile returns this image's config file.
func (i *pushableImage) ConfigFile() (*registryv1.ConfigFile, error) {
	return i.config, nil
}

// RawConfigFile returns the serialized bytes of ConfigFile().
func (i *pushableImage) RawConfigFile() ([]byte, error) {
	return i.rawConfig, nil
}

// Digest returns the sha256 of this image's manifest.
func (i *pushableImage) Digest() (registryv1.Hash, error) {
	return registryv1.Hash{
		Algorithm: "sha256",
		Hex:       fmt.Sprintf("%x", sha256.Sum256(i.rawManifest)),
	}, nil
}

// Manifest returns this image's Manifest object.
func (i *pushableImage) Manifest() (*registryv1.Manifest, error) {
	return i.manifest, nil
}

// RawManifest returns the serialized bytes of Manifest()
func (i *pushableImage) RawManifest() ([]byte, error) {
	return i.rawManifest, nil
}

// LayerByDigest returns a Layer for interacting with a particular layer of
// the image, looking it up by "digest" (the compressed hash).
func (i *pushableImage) LayerByDigest(wanted registryv1.Hash) (registryv1.Layer, error) {
	for _, layer := range i.layers {
		digest, err := layer.Digest()
		if err != nil {
			return nil, err
		}
		if wanted.Hex == digest.Hex && wanted.Algorithm == digest.Algorithm {
			return layer, nil
		}
	}
	return nil, fmt.Errorf("layer with digest %s not found", wanted.Hex)
}

// LayerByDiffID is an analog to LayerByDigest, looking up by "diff id"
// (the uncompressed hash).
func (i *pushableImage) LayerByDiffID(wanted registryv1.Hash) (registryv1.Layer, error) {
	for _, layer := range i.layers {
		diffID, err := layer.DiffID()
		if err != nil {
			return nil, err
		}
		if wanted.Hex == diffID.Hex && wanted.Algorithm == diffID.Algorithm {
			return layer, nil
		}
	}
	return nil, fmt.Errorf("layer with diffID %s not found", wanted.Hex)
}
