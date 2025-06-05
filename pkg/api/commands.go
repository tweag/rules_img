package api

const (
	PushCommand  = "push"
	PushMetadata = "push-metadata"
)

type Dispatch struct {
	Command string `json:"command"`
}
