package push

import (
	"fmt"
	"io"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/malt3/rules_img/src/api"
)

type remoteBlob struct {
	blobMeta   api.LayerMetadata
	remoteInfo RemoteBlobInfo
}

func newRemoteBlob(blobMeta api.LayerMetadata, remoteInfo RemoteBlobInfo) *remoteBlob {
	return &remoteBlob{
		blobMeta:   blobMeta,
		remoteInfo: remoteInfo,
	}
}

func (r *remoteBlob) Compressed() (io.ReadCloser, error) {
	if len(r.remoteInfo.OriginalBaseImageRegistries) == 0 {
		return nil, fmt.Errorf("no registries provided")
	}

	ref, err := name.NewDigest(fmt.Sprintf("%s/%s@%s", r.remoteInfo.OriginalBaseImageRegistries[0], r.remoteInfo.OriginalBaseImageRepository, r.blobMeta.Digest))
	if err != nil {
		return nil, fmt.Errorf("creating blob reference: %w", err)
	}
	layer, err := remote.Layer(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("getting layer: %w", err)
	}
	return layer.Compressed()
}
