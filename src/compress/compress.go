package compress

import (
	"encoding"
	"hash"
	"io"

	"github.com/malt3/rules_img/src/api"
)

type Options struct {
	// CompressionLevel is the level of compression to use.
	// nil means default.
	CompressionLevel *int
}

func (o *Options) Merge(others ...Options) {
	for _, other := range others {
		if other.CompressionLevel != nil {
			o.CompressionLevel = other.CompressionLevel
		}
	}
}

// Appender appends data to a compressed blob, while
// hashing the compressed and uncompressed data.
// It implements the io.Writer interface.
// It is resumable, meaning that it can be resumed from a previous state.
type Appender[H ResumableHash, C Compressor] struct {
	outerHash   H
	contentHash H
	compressor  C
	// pipelineWriter is the writer that pushes
	// uncompressed data into the compression
	// and hashing pipeline, which also keeps
	// track of the bytes appended.
	pipelineWriter *countingWriter
	outputWriter   *countingWriter
}

// New creates a Appender with initial state.
func New[H ResumableHash, C Compressor, HM hashMaker[H], CM compressorMaker[C]](output io.Writer, options ...Options) (Appender[H, C], error) {
	opts := Options{}
	opts.Merge(options...)

	var hashMaker HM
	outerHash := hashMaker.New()
	contentHash := hashMaker.New()
	outputWriter := &countingWriter{w: output}
	pipelineWriter, compress, err := setupWriterPipeline[C, CM](outputWriter, outerHash, contentHash, opts)
	if err != nil {
		return Appender[H, C]{}, err
	}

	return Appender[H, C]{
		outerHash:      outerHash,
		contentHash:    contentHash,
		compressor:     compress,
		pipelineWriter: &countingWriter{w: pipelineWriter},
		outputWriter:   outputWriter,
	}, nil
}

// Resume resumes the Appender from a previous state.
func Resume[H ResumableHash, C Compressor, HM hashMaker[H], CM compressorMaker[C]](state api.AppenderState, output io.Writer, options ...Options) (Appender[H, C], error) {
	opts := Options{}
	opts.Merge(options...)

	var hashMaker HM
	outerHash := hashMaker.New()
	contentHash := hashMaker.New()
	if err := outerHash.UnmarshalBinary(state.OuterHashState); err != nil {
		return Appender[H, C]{}, err
	}
	if err := contentHash.UnmarshalBinary(state.ContentHashState); err != nil {
		return Appender[H, C]{}, err
	}
	outputWriter := &countingWriter{w: output, n: state.CompressedSize}
	pipelineWriter, compress, err := setupWriterPipeline[C, CM](outputWriter, outerHash, contentHash, opts)
	if err != nil {
		return Appender[H, C]{}, err
	}

	return Appender[H, C]{
		outerHash:      outerHash,
		contentHash:    contentHash,
		compressor:     compress,
		pipelineWriter: &countingWriter{w: pipelineWriter, n: state.UncompressedSize},
		outputWriter:   outputWriter,
	}, nil
}

// Write writes data to the Appender.
func (a Appender[H, C]) Write(data []byte) (int, error) {
	return a.pipelineWriter.Write(data)
}

// Flush flushes the Appender (by flushing the compressor).
func (a Appender[H, C]) Flush() error {
	return a.compressor.Flush()
}

// Finalize finalizes the Appender and returns the state.
func (a Appender[H, C]) Finalize() (api.AppenderState, error) {
	// Closing the compressor flushes the data to the output
	// and ensures a trailer is written (if needed).
	if err := a.compressor.Close(); err != nil {
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

	state := api.AppenderState{
		OuterHashState:   outerHashState,
		OuterHash:        a.outerHash.Sum(nil),
		ContentHashState: contentHashState,
		ContentHash:      a.contentHash.Sum(nil),
		CompressedSize:   a.outputWriter.n,
		UncompressedSize: a.pipelineWriter.n,
	}
	return state, nil
}

func setupWriterPipeline[C Compressor, CM compressorMaker[C]](output io.Writer, outerHash, contentHash ResumableHash, opts Options) (io.Writer, C, error) {
	// Pipeline for writing data:
	// input: uncompressed data
	// write to inputTee
	// inputTee: tee to compressor and contentHash
	// compressor: compresses data and writes to outputTee
	// outputTee: tee to outerHash and output

	outputTee := io.MultiWriter(output, outerHash)
	var compressorMaker CM
	var compress C
	if opts.CompressionLevel != nil {
		var err error
		compress, err = compressorMaker.NewWriterLevel(outputTee, *opts.CompressionLevel)
		if err != nil {
			return nil, compress, err
		}
	} else {
		compress = compressorMaker.NewWriter(outputTee)
	}
	inputTee := io.MultiWriter(compress, contentHash)
	return inputTee, compress, nil
}

type ResumableHash interface {
	hash.Hash
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

type Compressor interface {
	Close() error
	Flush() error
	Write([]byte) (int, error)
}

type hashMaker[T ResumableHash] interface {
	New() T
}

type compressorMaker[T Compressor] interface {
	NewWriter(w io.Writer) T
	NewWriterLevel(w io.Writer, level int) (T, error)
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
