package compress

import (
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"

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

type ZstdMaker struct{}

func (ZstdMaker) NewWriter(w io.Writer) *zstd.Encoder {
	encoder, _ := zstd.NewWriter(w)
	return encoder
}

func (ZstdMaker) NewWriterLevel(w io.Writer, level int) (*zstd.Encoder, error) {
	return zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.EncoderLevel(level)))
}

func (ZstdMaker) Name() string {
	return "zstd"
}

func NewSHA256GzipAppender(w io.Writer, options ...Option) (Appender[*gzip.Writer], error) {
	return New[*gzip.Writer, SHA256Maker, GZipMaker](w, options...)
}

func ResumeSHA256GzipAppender(state api.AppenderState, w io.Writer, options ...Option) (Appender[*gzip.Writer], error) {
	return Resume[*gzip.Writer, SHA256Maker, GZipMaker](state, w, options...)
}

func NewSHA256ZstdAppender(w io.Writer, options ...Option) (Appender[*zstd.Encoder], error) {
	return New[*zstd.Encoder, SHA256Maker, ZstdMaker](w, options...)
}

func ResumeSHA256ZstdAppender(state api.AppenderState, w io.Writer, options ...Option) (Appender[*zstd.Encoder], error) {
	return Resume[*zstd.Encoder, SHA256Maker, ZstdMaker](state, w, options...)
}

func NewSHA256EstargzGzipTarAppender(w io.Writer, options ...Option) (*TarAppender[*EstargzWriter], error) {
	appender, err := NewTar[*EstargzWriter, SHA256Maker, EstargzGzipCompressorMaker](w, options...)
	if err != nil {
		return nil, err
	}
	return &appender, nil
}

func ResumeSHA256EstargzGzipTarAppender(state api.AppenderState, w io.Writer, options ...Option) (*TarAppender[*EstargzWriter], error) {
	appender, err := ResumeTar[*EstargzWriter, SHA256Maker, EstargzGzipCompressorMaker](state, w, options...)
	if err != nil {
		return nil, err
	}
	return &appender, nil
}

func NewSHA256EstargzZstdTarAppender(w io.Writer, options ...Option) (*TarAppender[*EstargzWriter], error) {
	appender, err := NewTar[*EstargzWriter, SHA256Maker, EstargzZstdCompressorMaker](w, options...)
	if err != nil {
		return nil, err
	}
	return &appender, nil
}

func ResumeSHA256EstargzZstdTarAppender(state api.AppenderState, w io.Writer, options ...Option) (*TarAppender[*EstargzWriter], error) {
	appender, err := ResumeTar[*EstargzWriter, SHA256Maker, EstargzZstdCompressorMaker](state, w, options...)
	if err != nil {
		return nil, err
	}
	return &appender, nil
}

func AppenderFactory(hashAlgorithm, compressionAlgorithm string, w io.Writer, options ...Option) (api.Appender, error) {
	switch {
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip":
		return NewSHA256GzipAppender(w, options...)
	case hashAlgorithm == "sha256" && compressionAlgorithm == "zstd":
		return NewSHA256ZstdAppender(w, options...)
	case hashAlgorithm == "sha256" && compressionAlgorithm == "uncompressed":
		return New[nopCompressor, SHA256Maker, UncompressedMaker](w, options...)
	}
	return nil, errors.New("unsupported hash or compression algorithm")
}

func ResumeFactory(hashAlgorithm, compressionAlgorithm string, state api.AppenderState, w io.Writer, options ...Option) (api.Appender, error) {
	switch {
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip":
		return ResumeSHA256GzipAppender(state, w, options...)
	case hashAlgorithm == "sha256" && compressionAlgorithm == "zstd":
		return ResumeSHA256ZstdAppender(state, w, options...)
	case hashAlgorithm == "sha256" && compressionAlgorithm == "uncompressed":
		return Resume[nopCompressor, SHA256Maker, UncompressedMaker](state, w, options...)
	}
	return nil, errors.New("unsupported hash or compression algorithm")
}

func TarAppenderFactory(hashAlgorithm, compressionAlgorithm string, seekable bool, w io.Writer, options ...Option) (api.TarAppender, error) {
	switch {
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip" && seekable:
		return NewSHA256EstargzGzipTarAppender(w, options...)
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip" && !seekable:
		appender, err := NewSHA256GzipAppender(w, options...)
		if err != nil {
			return nil, err
		}
		return appender.TarAppender(), nil
	case hashAlgorithm == "sha256" && compressionAlgorithm == "zstd" && seekable:
		return NewSHA256EstargzZstdTarAppender(w, options...)
	case hashAlgorithm == "sha256" && compressionAlgorithm == "zstd" && !seekable:
		appender, err := NewSHA256ZstdAppender(w, options...)
		if err != nil {
			return nil, err
		}
		return appender.TarAppender(), nil
	}
	return nil, errors.New("unsupported hash or compression algorithm")
}

func TarAppenderFactoryWithSeekableMode(hashAlgorithm, compressionAlgorithm string, seekableMode SeekableMode, w io.Writer, options ...Option) (api.TarAppender, error) {
	switch seekableMode {
	case SeekableNone:
		return TarAppenderFactory(hashAlgorithm, compressionAlgorithm, false, w, options...)
	case SeekableEStargz:
		return TarAppenderFactory(hashAlgorithm, compressionAlgorithm, true, w, options...)
	case SeekableSOCI:
		if compressionAlgorithm != "gzip" {
			return nil, fmt.Errorf("SOCI only supports gzip compression, got %s", compressionAlgorithm)
		}
		return NewSHA256SOCIGzipTarAppender(w, options...)
	default:
		return nil, fmt.Errorf("unknown seekable mode: %v", seekableMode)
	}
}

func NewSHA256SOCIGzipTarAppender(w io.Writer, options ...Option) (*TarAppender[*SOCIGzipWriter], error) {
	opts := defaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	sociOpts := SOCIOptions{
		SpanSize:     4 * 1024 * 1024, // 4 MiB default
		MinLayerSize: 10 * 1024 * 1024, // 10 MiB default
	}

	appender, err := NewTar[*SOCIGzipWriter, SHA256Maker, SOCIGzipMaker](w, options...)
	if err != nil {
		return nil, err
	}
	return &appender, nil
}

func ResumeSHA256SOCIGzipTarAppender(state api.AppenderState, w io.Writer, options ...Option) (*TarAppender[*SOCIGzipWriter], error) {
	appender, err := ResumeTar[*SOCIGzipWriter, SHA256Maker, SOCIGzipMaker](state, w, options...)
	if err != nil {
		return nil, err
	}
	return &appender, nil
}

func ResumeTarFactory(hashAlgorithm, compressionAlgorithm string, seekable bool, state api.AppenderState, w io.Writer, options ...Option) (api.TarAppender, error) {
	switch {
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip" && seekable:
		return ResumeSHA256EstargzGzipTarAppender(state, w, options...)
	case hashAlgorithm == "sha256" && compressionAlgorithm == "gzip" && !seekable:
		appender, err := ResumeSHA256GzipAppender(state, w, options...)
		if err != nil {
			return nil, err
		}
		return appender.TarAppender(), nil
	case hashAlgorithm == "sha256" && compressionAlgorithm == "zstd" && seekable:
		return ResumeSHA256EstargzZstdTarAppender(state, w, options...)
	case hashAlgorithm == "sha256" && compressionAlgorithm == "zstd" && !seekable:
		appender, err := ResumeSHA256ZstdAppender(state, w, options...)
		if err != nil {
			return nil, err
		}
		return appender.TarAppender(), nil
	}
	return nil, errors.New("unsupported hash or compression algorithm")
}
