package registry

import(
	"github.com/malt3/go-containerregistry/pkg/authn"
	"github.com/malt3/go-containerregistry/pkg/v1/google"
	"github.com/malt3/go-containerregistry/pkg/v1/remote"
)

func WithAuthFromMultiKeychain() remote.Option {
	kc := authn.NewMultiKeychain(
		authn.DefaultKeychain,
		google.Keychain,
	)

	return remote.WithAuthFromKeychain(kc)
}
