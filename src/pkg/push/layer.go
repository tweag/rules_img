package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/bazelbuild/rules_go/go/runfiles"
	registryv1 "github.com/malt3/go-containerregistry/pkg/v1"
	types "github.com/malt3/go-containerregistry/pkg/v1/types"

	"github.com/tweag/rules_img/src/pkg/api"
	"github.com/tweag/rules_img/src/pkg/cas"
)

type pushableLayer struct {
	blobPath string
	metadata api.Descriptor
	remote   compressedReader
}

func newMetadataLayer(blobMeta api.Descriptor, reader blobReader) *pushableLayer {
	return &pushableLayer{
		metadata: blobMeta,
		remote:   newCASBlob(blobMeta, reader),
	}
}

func newPushableLayer(input LayerInput, remoteInfo api.PullInfo) (*pushableLayer, error) {
	metadataPath, err := runfiles.Rlocation(input.Metadata)
	if err != nil {
		return nil, err
	}
	rawMetadata, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(rawMetadata))
	decoder.DisallowUnknownFields()
	var layerMetadata api.Descriptor
	if err := decoder.Decode(&layerMetadata); err != nil {
		return nil, err
	}
	var remote *remoteBlob
	if len(input.BlobPath) == 0 {
		remote = newRemoteBlob(layerMetadata, remoteInfo)
	}
	return &pushableLayer{
		blobPath: input.BlobPath,
		metadata: layerMetadata,
		remote:   remote,
	}, nil
}

// Digest returns the Hash of the compressed layer.
func (l *pushableLayer) Digest() (registryv1.Hash, error) {
	parts := strings.SplitN(l.metadata.Digest, ":", 2)
	if len(parts) != 2 {
		return registryv1.Hash{}, errors.New("invalid digest format")
	}
	return registryv1.Hash{
		Algorithm: parts[0],
		Hex:       parts[1],
	}, nil
}

// DiffID returns the Hash of the uncompressed layer.
func (l *pushableLayer) DiffID() (registryv1.Hash, error) {
	parts := strings.SplitN(l.metadata.DiffID, ":", 2)
	if len(parts) != 2 {
		return registryv1.Hash{}, errors.New("invalid diffID format")
	}
	return registryv1.Hash{
		Algorithm: parts[0],
		Hex:       parts[1],
	}, nil
}

// Compressed returns an io.ReadCloser for the compressed layer contents.
func (l *pushableLayer) Compressed() (io.ReadCloser, error) {
	if len(l.blobPath) > 0 {
		blobPath, err := runfiles.Rlocation(l.blobPath)
		if err != nil {
			return nil, err
		}
		return os.Open(blobPath)
	}
	if l.remote == nil {
		return nil, errors.New("no blob path or remote blob provided")
	}
	return l.remote.Compressed()
}

// Uncompressed returns an io.ReadCloser for the uncompressed layer contents.
func (l *pushableLayer) Uncompressed() (io.ReadCloser, error) {
	// Let's hope that we can get by without this for now.
	return nil, errors.New("Uncompressed() not implemented")
}

// Size returns the compressed size of the Layer.
func (l *pushableLayer) Size() (int64, error) {
	return l.metadata.Size, nil
}

// MediaType returns the media type of the Layer.
func (l *pushableLayer) MediaType() (types.MediaType, error) {
	return types.MediaType(l.metadata.MediaType), nil
}

type compressedReader interface {
	Compressed() (io.ReadCloser, error)
}

type blobReader interface {
	FindMissingBlobs(ctx context.Context, digests []cas.Digest) ([]cas.Digest, error)
	ReadBlob(ctx context.Context, digest cas.Digest) ([]byte, error)
	ReaderForBlob(ctx context.Context, digest cas.Digest) (io.ReadCloser, error)
}
