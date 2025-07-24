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
	casReader *cas.CAS
}

func newCASBlob(blobMeta api.Descriptor, casReader *cas.CAS) *casBlob {
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
	return r.casReader.ReaderForBlob(context.TODO(), digest)
}

func digestFromDescriptor(blobMeta api.Descriptor) (cas.Digest, error) {
	hash, err := registryv1.NewHash(blobMeta.Digest)
	if err != nil {
		return cas.Digest{}, fmt.Errorf("failed to parse digest: %w", err)
	}
	rawHash, err := hex.DecodeString(blobMeta.Digest)
	if err != nil {
		return cas.Digest{}, fmt.Errorf("failed to decode digest hash: %w", err)
	}

	switch hash.Algorithm {
	case "sha256":
		return cas.SHA256(rawHash, blobMeta.Size), nil
	case "sha512":
		return cas.SHA512(rawHash, blobMeta.Size), nil
	}
	return cas.Digest{}, fmt.Errorf("unsupported digest algorithm: %s", hash.Algorithm)
}
