package push

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"

	registryv1 "github.com/malt3/go-containerregistry/pkg/v1"

	"github.com/tweag/rules_img/pkg/api"
	"github.com/tweag/rules_img/pkg/cas"
)

// casBlob is a blob / layer that is backed by a CAS (Content Addressable Storage) system.
type casBlob struct {
	blobMeta  api.Descriptor
	casReader blobReader
}

func newCASBlob(blobMeta api.Descriptor, casReader blobReader) *casBlob {
	return &casBlob{
		blobMeta:  blobMeta,
		casReader: casReader,
	}
}

func (r *casBlob) Compressed() (io.ReadCloser, error) {
	digest, err := digestFromDescriptor(r.blobMeta)
	if err != nil {
		return nil, err
	}
	reader, err := r.casReader.ReaderForBlob(context.TODO(), digest)
	return reader, err
}

func digestFromDescriptor(blobMeta api.Descriptor) (cas.Digest, error) {
	hash, err := registryv1.NewHash(blobMeta.Digest)
	if err != nil {
		return cas.Digest{}, fmt.Errorf("failed to parse digest: %w", err)
	}
	return digestFromHasAndSize(hash, blobMeta.Size)
}

func digestFromV1Descriptor(blobMeta registryv1.Descriptor) (cas.Digest, error) {
	return digestFromHasAndSize(blobMeta.Digest, blobMeta.Size)
}

func digestFromHasAndSize(hash registryv1.Hash, sizeBytes int64) (cas.Digest, error) {
	rawHash, err := hex.DecodeString(hash.Hex)
	if err != nil {
		return cas.Digest{}, fmt.Errorf("failed to decode digest hash: %w", err)
	}

	switch hash.Algorithm {
	case "sha256":
		return cas.SHA256(rawHash, sizeBytes), nil
	case "sha512":
		return cas.SHA512(rawHash, sizeBytes), nil
	}
	return cas.Digest{}, fmt.Errorf("unsupported digest algorithm: %s", hash.Algorithm)
}
