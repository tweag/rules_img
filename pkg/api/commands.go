package api

const (
	PushCommand  = "push"
	PushMetadata = "push-metadata"
	LoadCommand  = "load"
)

type Dispatch struct {
	Command string `json:"command"`
}
