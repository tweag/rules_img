package load

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"

	registryname "github.com/malt3/go-containerregistry/pkg/name"
	registryv1 "github.com/malt3/go-containerregistry/pkg/v1"
	"github.com/malt3/go-containerregistry/pkg/v1/remote"

	"github.com/tweag/rules_img/pkg/api"
	"github.com/tweag/rules_img/pkg/cas"
	"github.com/tweag/rules_img/pkg/auth/registry"
)

type casBlob struct {
	blobMeta  api.Descriptor
	casReader BlobReader
}

func newCASBlob(blobMeta api.Descriptor, casReader BlobReader) *casBlob {
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

type remoteBlob struct {
	blobMeta   api.Descriptor
	remoteInfo api.PullInfo
}

func newRemoteBlob(blobMeta api.Descriptor, remoteInfo api.PullInfo) *remoteBlob {
	return &remoteBlob{
		blobMeta:   blobMeta,
		remoteInfo: remoteInfo,
	}
}

func (r *remoteBlob) Compressed() (io.ReadCloser, error) {
	if len(r.remoteInfo.OriginalBaseImageRegistries) == 0 {
		return nil, fmt.Errorf("no registries provided")
	}

	ref, err := registryname.NewDigest(fmt.Sprintf("%s/%s@%s", r.remoteInfo.OriginalBaseImageRegistries[0], r.remoteInfo.OriginalBaseImageRepository, r.blobMeta.Digest))
	if err != nil {
		return nil, fmt.Errorf("creating blob reference: %w", err)
	}
	layer, err := remote.Layer(ref, registry.WithAuthFromMultiKeychain())
	if err != nil {
		return nil, fmt.Errorf("getting layer: %w", err)
	}
	return layer.Compressed()
}

func GetCASLayerReader(desc api.Descriptor, casReader BlobReader) (io.ReadCloser, error) {
	return newCASBlob(desc, casReader).Compressed()
}

func GetRemoteLayerReader(desc api.Descriptor, remoteInfo api.PullInfo) (io.ReadCloser, error) {
	return newRemoteBlob(desc, remoteInfo).Compressed()
}

func digestFromDescriptor(blobMeta api.Descriptor) (cas.Digest, error) {
	hash, err := registryv1.NewHash(blobMeta.Digest)
	if err != nil {
		return cas.Digest{}, fmt.Errorf("failed to parse digest: %w", err)
	}
	return digestFromHashAndSize(hash, blobMeta.Size)
}

func digestFromHashAndSize(hash registryv1.Hash, sizeBytes int64) (cas.Digest, error) {
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
