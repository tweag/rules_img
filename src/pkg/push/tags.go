package push

import (
	"context"
	"fmt"

	"github.com/malt3/go-containerregistry/pkg/authn"
	"github.com/malt3/go-containerregistry/pkg/name"
	"github.com/malt3/go-containerregistry/pkg/v1/remote"
)

// PushTags tags an existing image (by digest) with the provided tags.
func PushTags(ctx context.Context, baseReference, digest string, tags []string) error {
	if len(tags) == 0 {
		// No tags to push, nothing to do
		return nil
	}

	digestRef, err := name.ParseReference(baseReference + "@" + digest)
	if err != nil {
		return fmt.Errorf("parsing digest reference: %w", err)
	}

	desc, err := remote.Get(digestRef, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return fmt.Errorf("getting image descriptor: %w", err)
	}

	for _, tag := range tags {
		tagRef, err := name.NewTag(baseReference + ":" + tag)
		if err != nil {
			return fmt.Errorf("parsing tag reference %s:%s: %w", baseReference, tag, err)
		}

		if err := remote.Tag(tagRef, desc, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
			return fmt.Errorf("tagging %s:%s: %w", baseReference, tag, err)
		}
	}

	return nil
}
