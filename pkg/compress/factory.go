package compress

import (
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"io"

	"github.com/tweag/rules_img/pkg/api"
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

type nopCompressor struct {
	underlying io.Writer
}

func (n nopCompressor) Close() error {
	return nil
}

func (n nopCompressor) Flush() error {
	return nil
}

func (n nopCompressor) Write(p []byte) (int, error) {
	return n.underlying.Write(p)
}

type UncompressedMaker struct{}

func (UncompressedMaker) NewWriter(w io.Writer) nopCompressor {
	return nopCompressor{underlying: w}
}

func (UncompressedMaker) NewWriterLevel(w io.Writer, level int) (nopCompressor, error) {
	return nopCompressor{underlying: w}, nil
}

func (UncompressedMaker) Name() string {
	return "uncompressed"
}

func NewSHA256GzipAppender(w io.Writer, options ...Option) (Appender[*gzip.Writer], error) {
	return New[*gzip.Writer, SHA256Maker, GZipMaker](w, options...)
}

func ResumeSHA256GzipAppender(state api.AppenderState, w io.Writer, options ...Option) (Appender[*gzip.Writer], error) {
	return Resume[*gzip.Writer, SHA256Maker, GZipMaker](state, w, options...)
}

func NewSHA256EstargzTarAppender(w io.Writer, options ...Option) (*TarAppender[*EstargzWriter], error) {
	appender, err := NewTar[*EstargzWriter, SHA256Maker, EstargzCompressorMaker](w, options...)
	if err != nil {
		return nil, err
	}
	return &appender, nil
}

func ResumeSHA256EstargzTarAppender(state api.AppenderState, w io.Writer, options ...Option) (*TarAppender[*EstargzWriter], error) {
	appender, err := ResumeTar[*EstargzWriter, SHA256Maker, EstargzCompressorMaker](state, w, options...)
	if err != nil {
		return nil, err
	}
	return &appender, nil
}

func AppenderFactory(hashAlgorithm, compressionAlgorithm string, w io.Writer, options ...Option) (api.Appender, error) {
	switch {
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip":
		return NewSHA256GzipAppender(w, options...)
	case hashAlgorithm == "sha256" && compressionAlgorithm == "uncompressed":
		return New[nopCompressor, SHA256Maker, UncompressedMaker](w, options...)
	}
	return nil, errors.New("unsupported hash or compression algorithm")
}

func ResumeFactory(hashAlgorithm, compressionAlgorithm string, state api.AppenderState, w io.Writer, options ...Option) (api.Appender, error) {
	switch {
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip":
		return ResumeSHA256GzipAppender(state, w, options...)
	case hashAlgorithm == "sha256" && compressionAlgorithm == "uncompressed":
		return Resume[nopCompressor, SHA256Maker, UncompressedMaker](state, w, options...)
	}
	return nil, errors.New("unsupported hash or compression algorithm")
}

func TarAppenderFactory(hashAlgorithm, compressionAlgorithm string, seekable bool, w io.Writer, options ...Option) (api.TarAppender, error) {
	switch {
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip" && seekable:
		return NewSHA256EstargzTarAppender(w, options...)
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip" && !seekable:
		appender, err := NewSHA256GzipAppender(w, options...)
		if err != nil {
			return nil, err
		}
		return appender.TarAppender(), nil
	}
	return nil, errors.New("unsupported hash or compression algorithm")
}

func ResumeTarFactory(hashAlgorithm, compressionAlgorithm string, seekable bool, state api.AppenderState, w io.Writer, options ...Option) (api.TarAppender, error) {
	switch {
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip" && seekable:
		return ResumeSHA256EstargzTarAppender(state, w, options...)
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip" && !seekable:
		appender, err := ResumeSHA256GzipAppender(state, w, options...)
		if err != nil {
			return nil, err
		}
		return appender.TarAppender(), nil
	}
	return nil, errors.New("unsupported hash or compression algorithm")
}
