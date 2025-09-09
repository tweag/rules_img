package tarcas

import (
	"crypto/sha256"
	"errors"
	"hash"

	"github.com/tweag/rules_img/src/pkg/api"
)

type SHA256Helper struct{}

func (SHA256Helper) New() hash.Hash {
	return sha256.New()
}

func NewSHA256CAS(appender api.TarAppender, options ...Option) *CAS[SHA256Helper] {
	return New[SHA256Helper](appender, options...)
}

func CASFactory(hashAlgorithm string, appender api.TarAppender, options ...Option) (api.TarCAS, error) {
	switch {
	case hashAlgorithm == "sha256":
		return NewSHA256CAS(appender, options...), nil
	}
	return nil, errors.New("unsupported hash algorithm")
}
