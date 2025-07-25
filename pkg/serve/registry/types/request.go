package registrytypes

import (
	"github.com/tweag/rules_img/pkg/api"
)

type PushRequest struct {
	Strategy     string           `json:"strategy,omitempty"`
	Blobs        []api.Descriptor `json:"blobs"`
	MissingBlobs []string         `json:"missing_blobs,omitempty"`
	api.PushTarget
	api.PullInfo
}
