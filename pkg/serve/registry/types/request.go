package registrytypes

import (
	registryv1 "github.com/malt3/go-containerregistry/pkg/v1"

	"github.com/tweag/rules_img/pkg/api"
)

type PushRequest struct {
	Blobs []registryv1.Descriptor `json:"blobs"`
	api.PushTarget
	api.PullInfo
}
