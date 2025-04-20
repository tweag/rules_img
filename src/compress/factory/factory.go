package factory

import (
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"io"

	"github.com/malt3/rules_img/src/api"
	"github.com/malt3/rules_img/src/compress"
)

type SHA256Maker struct{}

func (SHA256Maker) New() compress.ResumableHash {
	h := sha256.New()
	return h.(compress.ResumableHash)
}

type GZipMaker struct{}

func (GZipMaker) NewWriter(w io.Writer) *gzip.Writer {
	return gzip.NewWriter(w)
}

func (GZipMaker) NewWriterLevel(w io.Writer, level int) (*gzip.Writer, error) {
	return gzip.NewWriterLevel(w, level)
}

func NewSHA256GzipAppender(w io.Writer, options ...compress.Options) (compress.Appender[compress.ResumableHash, *gzip.Writer], error) {
	return compress.New[compress.ResumableHash, *gzip.Writer, SHA256Maker, GZipMaker](w, options...)
}

func ResumeSHA256GzipAppender(state api.AppenderState, w io.Writer, options ...compress.Options) (compress.Appender[compress.ResumableHash, *gzip.Writer], error) {
	return compress.Resume[compress.ResumableHash, *gzip.Writer, SHA256Maker, GZipMaker](state, w, options...)
}

func AppenderFactory(hashAlgorithm, compressionAlgorithm string, w io.Writer, options ...compress.Options) (api.Appender, error) {
	switch {
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip":
		return NewSHA256GzipAppender(w, options...)
	}
	return nil, errors.New("unsupported hash or compression algorithm")
}

func ResumeFactory(hashAlgorithm, compressionAlgorithm string, state api.AppenderState, w io.Writer, options ...compress.Options) (api.Appender, error) {
	switch {
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip":
		return ResumeSHA256GzipAppender(state, w, options...)
	}
	return nil, errors.New("unsupported hash or compression algorithm")
}
