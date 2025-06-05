package push

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/bazelbuild/rules_go/go/runfiles"
	registryv1 "github.com/google/go-containerregistry/pkg/v1"
	types "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/tweag/rules_img/pkg/api"
)

type pushableLayer struct {
	blobPath string
	metadata api.LayerMetadata
	remote   *remoteBlob
}

func newPushableLayer(input LayerInput, remoteInfo RemoteBlobInfo) (*pushableLayer, error) {
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
	var layerMetadata api.LayerMetadata
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
