package compress

import (
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"io"

	"github.com/malt3/rules_img/src/api"
)

type SHA256Maker struct{}

func (SHA256Maker) New() ResumableHash {
	h := sha256.New()
	return h.(ResumableHash)
}

func (SHA256Maker) Name() string {
	return "sha256"
}

type GZipMaker struct{}

func (GZipMaker) NewWriter(w io.Writer) *gzip.Writer {
	return gzip.NewWriter(w)
}

func (GZipMaker) NewWriterLevel(w io.Writer, level int) (*gzip.Writer, error) {
	return gzip.NewWriterLevel(w, level)
}

func (GZipMaker) Name() string {
	return "gzip"
}

func NewSHA256GzipAppender(w io.Writer, options ...Option) (Appender[*gzip.Writer], error) {
	return New[*gzip.Writer, SHA256Maker, GZipMaker](w, options...)
}

func ResumeSHA256GzipAppender(state api.AppenderState, w io.Writer, options ...Option) (Appender[*gzip.Writer], error) {
	return Resume[*gzip.Writer, SHA256Maker, GZipMaker](state, w, options...)
}

func AppenderFactory(hashAlgorithm, compressionAlgorithm string, w io.Writer, options ...Option) (api.Appender, error) {
	switch {
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip":
		return NewSHA256GzipAppender(w, options...)
	}
	return nil, errors.New("unsupported hash or compression algorithm")
}

func ResumeFactory(hashAlgorithm, compressionAlgorithm string, state api.AppenderState, w io.Writer, options ...Option) (api.Appender, error) {
	switch {
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip":
		return ResumeSHA256GzipAppender(state, w, options...)
	}
	return nil, errors.New("unsupported hash or compression algorithm")
}
