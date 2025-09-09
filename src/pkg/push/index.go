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

type pushableIndex struct {
	rawIndex  []byte
	index     *registryv1.IndexManifest
	manifests []*pushableImage
}

func newPushableIndex(req PushIndexRequest) (*pushableIndex, error) {
	indexPath, err := runfiles.Rlocation(req.IndexPath)
	if err != nil {
		return nil, fmt.Errorf("reading index file: %w", err)
	}
	rawIndex, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}
	var index registryv1.IndexManifest
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		return nil, err
	}
	manifests := make([]*pushableImage, len(req.ManifestRequests))
	for i, manifestReq := range req.ManifestRequests {
		manifest, err := newPushableImage(manifestReq)
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

// MediaType of this image's manifest.
func (i *pushableIndex) MediaType() (types.MediaType, error) {
	return i.index.MediaType, nil
}

// Digest returns the sha256 of this index's manifest.
func (i *pushableIndex) Digest() (registryv1.Hash, error) {
	digest := sha256.Sum256(i.rawIndex)
	return registryv1.Hash{
		Algorithm: "sha256",
		Hex:       fmt.Sprintf("%x", digest),
	}, nil
}

// Size returns the size of the manifest.
func (i *pushableIndex) Size() (int64, error) {
	return int64(len(i.rawIndex)), nil
}

// IndexManifest returns this image index's manifest object.
func (i *pushableIndex) IndexManifest() (*registryv1.IndexManifest, error) {
	return i.index, nil
}

// RawManifest returns the serialized bytes of IndexManifest().
func (i *pushableIndex) RawManifest() ([]byte, error) {
	return i.rawIndex, nil
}

// Image returns a v1.Image that this ImageIndex references.
func (i *pushableIndex) Image(wanted registryv1.Hash) (registryv1.Image, error) {
	for _, manifest := range i.manifests {
		digest, err := manifest.Digest()
		if err != nil {
			return nil, err
		}
		if wanted.Hex == digest.Hex && wanted.Algorithm == digest.Algorithm {
			return manifest, nil
		}
	}
	return nil, fmt.Errorf("image with digest %s not found", wanted.Hex)
}

// ImageIndex returns a v1.ImageIndex that this ImageIndex references.
func (i *pushableIndex) ImageIndex(wanted registryv1.Hash) (registryv1.ImageIndex, error) {
	return i, nil
}
