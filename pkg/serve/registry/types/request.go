package registrytypes

import (
	"github.com/tweag/rules_img/pkg/api"
)

type PushRequest struct {
	Strategy string           `json:"strategy,omitempty"`
	Blobs    []api.Descriptor `json:"blobs"`
	api.PushTarget
	api.PullInfo
}
