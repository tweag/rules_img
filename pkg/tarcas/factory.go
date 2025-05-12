package tarcas

import (
	"crypto/sha256"
	"errors"
	"hash"
	"io"

	"github.com/malt3/rules_img/pkg/api"
)

type SHA256Helper struct{}

func (SHA256Helper) New() hash.Hash {
	return sha256.New()
}

func NewSHA256CAS(w io.Writer, options ...Option) *CAS[SHA256Helper] {
	return New[SHA256Helper](w, options...)
}

func CASFactory(hashAlgorithm string, w io.Writer, options ...Option) (api.TarCAS, error) {
	switch {
	case hashAlgorithm == "sha256":
		return NewSHA256CAS(w, options...), nil
	}
	return nil, errors.New("unsupported hash algorithm")
}
