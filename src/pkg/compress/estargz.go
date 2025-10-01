package compress

import (
	"fmt"
	"io"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/containerd/stargz-snapshotter/estargz/zstdchunked"
	"github.com/klauspost/compress/zstd"

	"github.com/bazel-contrib/rules_img/src/pkg/api"
)

// TarAppender appends tar entries to a compressed blob using estargz,
// while hashing the compressed and uncompressed data.
// It is resumable, meaning that it can be resumed from a previous state.
type TarAppender[C TarCompressor] struct {
	hashFunctionName         string
	compressionAlgorithmName string
	outerHash                ResumableHash
	contentHash              ResumableHash
	compressor               C
	outputWriter             *countingWriter
	uncompressedSize         int64
	options
}

// NewTar creates a TarAppender with initial state.
func NewTar[C TarCompressor, HM hashMaker, CM tarCompressorMaker[C]](output io.Writer, opts ...Option) (TarAppender[C], error) {
	options := options{}
	for _, opt := range opts {
		opt.apply(&options)
	}

	var hashMaker HM
	outerHash := hashMaker.New()
	contentHash := hashMaker.New()
	outputWriter := &countingWriter{w: output}

	var compressorMaker CM
	compress, err := setupTarWriterPipeline[C, CM](outputWriter, outerHash, contentHash, options)
	if err != nil {
		return TarAppender[C]{}, err
	}

	return TarAppender[C]{
		hashFunctionName:         hashMaker.Name(),
		compressionAlgorithmName: compressorMaker.Name(),
		outerHash:                outerHash,
		contentHash:              contentHash,
		compressor:               compress,
		outputWriter:             outputWriter,
		uncompressedSize:         0,
		options:                  options,
	}, nil
}

// ResumeTar resumes the TarAppender from a previous state.
func ResumeTar[C TarCompressor, HM hashMaker, CM tarCompressorMaker[C]](state api.AppenderState, output io.Writer, opts ...Option) (TarAppender[C], error) {
	options := options{}
	for _, opt := range opts {
		opt.apply(&options)
	}

	var hashMaker HM
	outerHash := hashMaker.New()
	contentHash := hashMaker.New()
	if err := outerHash.UnmarshalBinary(state.OuterHashState); err != nil {
		return TarAppender[C]{}, err
	}
	if err := contentHash.UnmarshalBinary(state.ContentHashState); err != nil {
		return TarAppender[C]{}, err
	}
	outputWriter := &countingWriter{w: output, n: state.CompressedSize}

	var compressorMaker CM
	compress, err := setupTarWriterPipeline[C, CM](outputWriter, outerHash, contentHash, options)
	if err != nil {
		return TarAppender[C]{}, err
	}

	appender := TarAppender[C]{
		hashFunctionName:         hashMaker.Name(),
		compressionAlgorithmName: compressorMaker.Name(),
		outerHash:                outerHash,
		contentHash:              contentHash,
		compressor:               compress,
		outputWriter:             outputWriter,
		uncompressedSize:         state.UncompressedSize,
		options:                  options,
	}

	if state.Magic != appender.magic() {
		return TarAppender[C]{}, fmt.Errorf("magic mismatch: expected %s, got %s", appender.magic(), state.Magic)
	}

	return appender, nil
}

// AppendTar appends a tar entry to the TarAppender using estargz Writer.AppendTar.
func (a *TarAppender[C]) AppendTar(r io.Reader) error {
	// Create a tee reader to hash the content and track size
	contentReader := io.TeeReader(r, a.contentHash)
	countingReader := &countingReader{r: contentReader}

	if err := a.compressor.AppendTar(countingReader); err != nil {
		return err
	}

	a.uncompressedSize += countingReader.n
	return nil
}

// Finalize finalizes the TarAppender and returns the state.
func (a *TarAppender[C]) Finalize() (api.AppenderState, error) {
	// Closing the compressor flushes the data to the output
	// and ensures a trailer is written (if needed).
	tocDigest, err := a.compressor.Close()
	if err != nil {
		return api.AppenderState{}, err
	}

	outerHashState, err := a.outerHash.MarshalBinary()
	if err != nil {
		return api.AppenderState{}, err
	}
	contentHashState, err := a.contentHash.MarshalBinary()
	if err != nil {
		return api.AppenderState{}, err
	}

	// Populate layer annotations for estargz
	layerAnnotations := make(map[string]string)
	layerAnnotations[api.TocDigestAnnotation] = tocDigest
	layerAnnotations[api.UncompressedSizeAnnotation] = fmt.Sprintf("%d", a.uncompressedSize)

	state := api.AppenderState{
		Magic:            a.magic(),
		OuterHashState:   outerHashState,
		OuterHash:        a.outerHash.Sum(nil),
		ContentHashState: contentHashState,
		ContentHash:      a.contentHash.Sum(nil),
		CompressedSize:   a.outputWriter.n,
		UncompressedSize: a.uncompressedSize,
		LayerAnnotations: layerAnnotations,
	}
	return state, nil
}

func (a *TarAppender[C]) magic() string {
	magic := fmt.Sprintf("imgv1+tar+compressed+%s+%s", a.hashFunctionName, a.compressionAlgorithmName)
	if len(a.contentType) > 0 {
		magic += "+" + string(a.contentType)
	}
	return magic
}

func setupTarWriterPipeline[C TarCompressor, CM tarCompressorMaker[C]](output io.Writer, outerHash, contentHash ResumableHash, opts options) (C, error) {
	// Pipeline for writing data:
	// estargz writer -> output tee (to outerHash and final output)
	outputTee := io.MultiWriter(output, outerHash)

	var compressorMaker CM
	var compress C
	if opts.compressionLevel != nil {
		var err error
		compress, err = compressorMaker.NewWriterLevel(outputTee, int(*opts.compressionLevel))
		if err != nil {
			return compress, err
		}
	} else {
		compress = compressorMaker.NewWriter(outputTee)
	}

	return compress, nil
}

// TarCompressor interface for estargz-based compression
type TarCompressor interface {
	AppendTar(r io.Reader) error
	Close() (string, error)
}

type tarCompressorMaker[T TarCompressor] interface {
	NewWriter(w io.Writer) T
	NewWriterLevel(w io.Writer, level int) (T, error)
	Name() string
}

// countingReader tracks the number of bytes read
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// EstargzWriter wraps estargz.Writer to implement TarCompressor
type EstargzWriter struct {
	writer            *estargz.Writer
	compressionFormat string
}

// NewEstargzWriter creates a new EstargzWriter with default gzip compression
func NewEstargzWriter(w io.Writer) *EstargzWriter {
	writer := estargz.NewWriter(w)
	return &EstargzWriter{writer: writer, compressionFormat: "gzip"}
}

// NewEstargzWriterLevel creates a new EstargzWriter with compression level
func NewEstargzWriterLevel(w io.Writer, level int) (*EstargzWriter, error) {
	compressor := estargz.NewGzipCompressorWithLevel(level)
	writer := estargz.NewWriterWithCompressor(w, compressor)
	return &EstargzWriter{writer: writer, compressionFormat: "gzip"}, nil
}

// NewEstargzWriterWithCompression creates a new EstargzWriter with specified compression
func NewEstargzWriterWithCompression(w io.Writer, compressionFormat string, level int) (*EstargzWriter, error) {
	var compressor estargz.Compressor
	switch compressionFormat {
	case "gzip":
		compressor = estargz.NewGzipCompressorWithLevel(level)
	case "zstd":
		compressor = &zstdchunked.Compressor{
			CompressionLevel: zstd.EncoderLevel(level),
			Metadata:         make(map[string]string),
		}
	default:
		return nil, fmt.Errorf("unsupported compression format: %s", compressionFormat)
	}
	writer := estargz.NewWriterWithCompressor(w, compressor)
	return &EstargzWriter{writer: writer, compressionFormat: compressionFormat}, nil
}

// AppendTar appends a tar entry using estargz Writer.AppendTar
func (e *EstargzWriter) AppendTar(r io.Reader) error {
	return e.writer.AppendTar(r)
}

// Close closes the estargz writer
func (e *EstargzWriter) Close() (string, error) {
	digest, err := e.writer.Close()
	return digest.String(), err
}

// EstargzGzipCompressorMaker implements tarCompressorMaker for EstargzWriter with gzip
type EstargzGzipCompressorMaker struct{}

func (EstargzGzipCompressorMaker) NewWriter(w io.Writer) *EstargzWriter {
	return NewEstargzWriter(w)
}

func (EstargzGzipCompressorMaker) NewWriterLevel(w io.Writer, level int) (*EstargzWriter, error) {
	return NewEstargzWriterLevel(w, level)
}

func (EstargzGzipCompressorMaker) Name() string {
	return "gzip"
}

// EstargzZstdCompressorMaker implements tarCompressorMaker for EstargzWriter with zstd
type EstargzZstdCompressorMaker struct{}

func (EstargzZstdCompressorMaker) NewWriter(w io.Writer) *EstargzWriter {
	writer, _ := NewEstargzWriterWithCompression(w, "zstd", 3) // default level
	return writer
}

func (EstargzZstdCompressorMaker) NewWriterLevel(w io.Writer, level int) (*EstargzWriter, error) {
	return NewEstargzWriterWithCompression(w, "zstd", level)
}

func (EstargzZstdCompressorMaker) Name() string {
	return "zstd"
}
