package push

import (
	"fmt"
	"io"

	"github.com/malt3/go-containerregistry/pkg/name"
	"github.com/malt3/go-containerregistry/pkg/v1/remote"

	"github.com/tweag/rules_img/pkg/api"
	"github.com/tweag/rules_img/pkg/auth/registry"
)

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

	ref, err := name.NewDigest(fmt.Sprintf("%s/%s@%s", r.remoteInfo.OriginalBaseImageRegistries[0], r.remoteInfo.OriginalBaseImageRepository, r.blobMeta.Digest))
	if err != nil {
		return nil, fmt.Errorf("creating blob reference: %w", err)
	}
	layer, err := remote.Layer(ref, registry.WithAuthFromMultiKeychain())
	if err != nil {
		return nil, fmt.Errorf("getting layer: %w", err)
	}
	return layer.Compressed()
}
