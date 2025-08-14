package soci

import (
	"encoding/json"
	"fmt"
	"time"

	v1 "github.com/malt3/go-containerregistry/pkg/v1"
	"github.com/malt3/go-containerregistry/pkg/v1/empty"
	"github.com/malt3/go-containerregistry/pkg/v1/mutate"
	"github.com/malt3/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// SOCIIndexMediaType is the media type for SOCI index manifests
	SOCIIndexMediaType = "application/vnd.amazon.soci.index.v2+json"

	// ZtocMediaType is the media type for ztoc blobs
	ZtocMediaType = "application/vnd.amazon.soci.ztoc.v1+binary"

	// LayerAnnotation marks which layer a ztoc belongs to
	LayerAnnotation = "com.amazon.soci.layer.digest"

	// IndexDigestAnnotation links an image manifest to its SOCI index
	IndexDigestAnnotation = "com.amazon.soci.index-digest"

	// ImageManifestDigestAnnotation links a SOCI index to its image manifest
	ImageManifestDigestAnnotation = "com.amazon.soci.image-manifest-digest"
)

// ZtocDescriptor holds information about a ztoc blob
type ZtocDescriptor struct {
	Digest      string
	Size        int64
	LayerDigest string
}

// BuildSOCIIndex creates a SOCI index manifest from ztoc descriptors
func BuildSOCIIndex(imageManifestDesc ocispec.Descriptor, ztocs []ZtocDescriptor) (v1.ImageIndex, error) {
	// Create empty index
	index := empty.Index

	// Set media type to SOCI index
	index = mutate.IndexMediaType(index, types.MediaType(SOCIIndexMediaType))

	// Set subject to point to the image manifest
	subject := v1.Descriptor{
		MediaType: types.MediaType(imageManifestDesc.MediaType),
		Size:      imageManifestDesc.Size,
		Digest:    v1.Hash{
			Algorithm: "sha256",
			Hex:       imageManifestDesc.Digest.Encoded(),
		},
	}

	// Add subject via config (workaround for go-containerregistry)
	config := v1.ConfigFile{
		Created: v1.Time{Time: time.Now()},
		Config: v1.Config{
			Labels: map[string]string{
				"artifactType": SOCIIndexMediaType,
			},
		},
	}

	// Create manifest with subject
	manifest := v1.Manifest{
		SchemaVersion: 2,
		MediaType:     types.MediaType(SOCIIndexMediaType),
		Config: v1.Descriptor{
			MediaType: types.MediaType(SOCIIndexMediaType),
			Size:      0,
			Digest:    v1.Hash{},
		},
		Subject: &subject,
	}

	// Add ztoc layers
	for _, ztoc := range ztocs {
		layer := v1.Descriptor{
			MediaType: types.MediaType(ZtocMediaType),
			Size:      ztoc.Size,
			Digest: v1.Hash{
				Algorithm: "sha256",
				Hex:       ztoc.Digest[len("sha256:"):], // Remove prefix
			},
			Annotations: map[string]string{
				LayerAnnotation: ztoc.LayerDigest,
			},
		}
		manifest.Layers = append(manifest.Layers, layer)
	}

	// Convert to partial.Describable
	return &sociIndex{manifest: manifest}, nil
}

// sociIndex implements v1.ImageIndex for SOCI
type sociIndex struct {
	manifest v1.Manifest
}

func (s *sociIndex) MediaType() (types.MediaType, error) {
	return types.MediaType(SOCIIndexMediaType), nil
}

func (s *sociIndex) Digest() (v1.Hash, error) {
	// Calculate digest of the manifest
	raw, err := s.RawManifest()
	if err != nil {
		return v1.Hash{}, err
	}
	return v1.Hash{
		Algorithm: "sha256",
		Hex:       digest.FromBytes(raw).Encoded(),
	}, nil
}

func (s *sociIndex) Size() (int64, error) {
	raw, err := s.RawManifest()
	if err != nil {
		return 0, err
	}
	return int64(len(raw)), nil
}

func (s *sociIndex) IndexManifest() (*v1.IndexManifest, error) {
	// Convert our manifest to IndexManifest format
	// This is a bit of a hack but works for our purposes
	return nil, fmt.Errorf("SOCI index does not support IndexManifest()")
}

func (s *sociIndex) RawManifest() ([]byte, error) {
	return json.Marshal(s.manifest)
}

func (s *sociIndex) Image(h v1.Hash) (v1.Image, error) {
	return nil, fmt.Errorf("SOCI index does not contain images")
}

func (s *sociIndex) ImageIndex(h v1.Hash) (v1.ImageIndex, error) {
	return nil, fmt.Errorf("SOCI index does not contain nested indexes")
}

// BuildBindingIndex creates an OCI image index that binds an image manifest and SOCI index
func BuildBindingIndex(imageManifest ocispec.Descriptor, sociIndex ocispec.Descriptor) (v1.ImageIndex, error) {
	// Create empty index
	index := empty.Index

	// Add image manifest descriptor with SOCI annotation
	imageDesc := v1.Descriptor{
		MediaType: types.MediaType(imageManifest.MediaType),
		Size:      imageManifest.Size,
		Digest: v1.Hash{
			Algorithm: "sha256",
			Hex:       imageManifest.Digest.Encoded(),
		},
		Platform: imageManifest.Platform,
		Annotations: map[string]string{
			IndexDigestAnnotation: sociIndex.Digest.String(),
		},
	}

	// Add SOCI index descriptor with image annotation
	sociDesc := v1.Descriptor{
		MediaType: types.MediaType(sociIndex.MediaType),
		Size:      sociIndex.Size,
		Digest: v1.Hash{
			Algorithm: "sha256",
			Hex:       sociIndex.Digest.Encoded(),
		},
		Annotations: map[string]string{
			ImageManifestDigestAnnotation: imageManifest.Digest.String(),
		},
		ArtifactType: SOCIIndexMediaType,
	}

	// Add both to the index
	index = mutate.AppendManifests(index, mutate.IndexAddendum{
		Add:        imageDesc,
		Descriptor: imageDesc,
	})

	index = mutate.AppendManifests(index, mutate.IndexAddendum{
		Add:        sociDesc,
		Descriptor: sociDesc,
	})

	return index, nil
}

// GetManifest returns the underlying manifest for pushing
func (s *sociIndex) GetManifest() (*v1.Manifest, error) {
	return &s.manifest, nil
}
