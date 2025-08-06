package push

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/malt3/go-containerregistry/pkg/authn"
	"github.com/malt3/go-containerregistry/pkg/name"
	registryv1 "github.com/malt3/go-containerregistry/pkg/v1"
	"github.com/malt3/go-containerregistry/pkg/v1/remote"

	"github.com/tweag/rules_img/pkg/api"
)

type LayerInput struct {
	Metadata string `json:"metadata"`
	BlobPath string `json:"blob_path"`
}

type PushManifestRequest struct {
	ManifestPath   string
	ConfigPath     string
	Layers         []LayerInput
	MissingBlobs   []string
	RemoteBlobInfo api.PullInfo
}

type PushIndexRequest struct {
	IndexPath        string
	ManifestRequests []PushManifestRequest
}

type eagerPusher struct{}

func New() *eagerPusher {
	return &eagerPusher{}
}

func (p *eagerPusher) PushManifest(ctx context.Context, reference string, req PushManifestRequest) (string, error) {
	manifest, err := newPushableImage(req)
	if err != nil {
		return "", err
	}
	digest, err := manifest.Digest()
	if err != nil {
		return "", err
	}
	reference = fmt.Sprintf("%s@%s", reference, digest.String())
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
	return digest.String(), nil
}

func (p *eagerPusher) PushIndex(ctx context.Context, reference string, req PushIndexRequest) (string, error) {
	index, err := newPushableIndex(req)
	if err != nil {
		return "", err
	}
	digest, err := index.Digest()
	if err != nil {
		return "", err
	}
	reference = fmt.Sprintf("%s@%s", reference, digest.String())
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
	return digest.String(), nil
}

func progressPrinter(updates <-chan registryv1.Update) {
	var lastUpdate time.Time
	for update := range updates {
		relative := float64(update.Complete) / float64(update.Total) * 100
		if update.Error != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", update.Error)
			continue
		}
		if time.Since(lastUpdate) < 10*time.Millisecond {
			// Avoid printing too frequently
			continue
		}
		fmt.Fprintf(os.Stderr, "Progress: %.2f %% (%v / %v bytes)\r", relative, update.Complete, update.Total)
		lastUpdate = time.Now()
	}
}
