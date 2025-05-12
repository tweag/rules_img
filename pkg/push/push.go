package push

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	registryv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type LayerInput struct {
	Metadata string `json:"metadata"`
	BlobPath string `json:"blob_path"`
}

type RemoteBlobInfo struct {
	OriginalBaseImageRegistries []string `json:"original_registries,omitempty"`
	OriginalBaseImageRepository string   `json:"original_repository,omitempty"`
	OriginalBaseImageTag        string   `json:"original_tag,omitempty"`
	OriginalBaseImageDigest     string   `json:"original_digest,omitempty"`
}

type PushManifestRequest struct {
	ManifestPath   string
	ConfigPath     string
	Layers         []LayerInput
	MissingBlobs   []string
	RemoteBlobInfo RemoteBlobInfo
}

type PushIndexRequest struct {
	IndexPath        string
	ManifestRequests []PushManifestRequest
}

type pusher struct{}

func New() *pusher {
	return &pusher{}
}

func (p *pusher) PushManifest(ctx context.Context, reference string, req PushManifestRequest) (string, error) {
	manifest, err := newPushableImage(req)
	if err != nil {
		return "", err
	}
	ref, err := name.ParseReference(reference)
	if err != nil {
		return "", err
	}
	updateChan := make(chan registryv1.Update, 64)
	go progressPrinter(updateChan)
	opts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithProgress(updateChan),
	}
	if err := remote.Write(ref, manifest, opts...); err != nil {
		return "", err
	}
	digest, err := manifest.Digest()
	if err != nil {
		return "", err
	}
	return digest.String(), nil
}

func (p *pusher) PushIndex(ctx context.Context, reference string, req PushIndexRequest) (string, error) {
	index, err := newPushableIndex(req)
	if err != nil {
		return "", err
	}
	ref, err := name.ParseReference(reference)
	if err != nil {
		return "", err
	}
	updateChan := make(chan registryv1.Update, 64)
	go progressPrinter(updateChan)
	opts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithProgress(updateChan),
	}
	if err := remote.WriteIndex(ref, index, opts...); err != nil {
		return "", err
	}
	digest, err := index.Digest()
	if err != nil {
		return "", err
	}
	return digest.String(), nil
}

func progressPrinter(updates <-chan registryv1.Update) {
	for update := range updates {
		relative := float64(update.Complete) / float64(update.Total) * 100
		if update.Error != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", update.Error)
			continue
		}
		fmt.Fprintf(os.Stderr, "Progress: %.2f %% (%v / %v bytes)\r", relative, update.Complete, update.Total)
	}
}
