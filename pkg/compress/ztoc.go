package compress

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"sync"

	"github.com/awslabs/soci-snapshotter/ztoc"
	"github.com/awslabs/soci-snapshotter/ztoc/compression"
	digest "github.com/opencontainers/go-digest"
)

type ZtocInfo struct {
	Digest      string
	Size        int64
	Bytes       []byte
	LayerDigest string
}

type sociGzipWriter struct {
	mu              sync.Mutex
	underlying      io.Writer
	gzipWriter      *gzip.Writer
	tocBuilder      *ztoc.TOCBuilder
	gzipZinfoWriter *compression.GzipZinfoWriter
	tarOffset       int64
	layerHasher     hash.Hash
	ztocInfo        *ZtocInfo
	opts            SOCIOptions
	closed          bool
}

func newSOCIGzipWriter(w io.Writer, opts SOCIOptions) (*sociGzipWriter, error) {
	if opts.SpanSize == 0 {
		opts.SpanSize = 4 * 1024 * 1024 // 4 MiB default
	}
	if opts.MinLayerSize == 0 {
		opts.MinLayerSize = 10 * 1024 * 1024 // 10 MiB default
	}

	gzipWriter := gzip.NewWriter(w)

	// Create a multiwriter to write to both the underlying writer and hash
	layerHasher := sha256.New()
	multiWriter := io.MultiWriter(w, layerHasher)

	// Create the gzip zinfo writer for tracking compression spans
	gzipZinfoWriter, err := compression.NewGzipZinfoWriter(io.Discard, opts.SpanSize, gzipWriter)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip zinfo writer: %w", err)
	}

	return &sociGzipWriter{
		underlying:      multiWriter,
		gzipWriter:      gzipWriter,
		tocBuilder:      ztoc.NewTOCBuilder(),
		gzipZinfoWriter: gzipZinfoWriter,
		layerHasher:     layerHasher,
		opts:            opts,
	}, nil
}

func (w *sociGzipWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, fmt.Errorf("writer is closed")
	}

	// Write to both gzip and zinfo writer
	n, err = w.gzipWriter.Write(p)
	if err != nil {
		return n, err
	}

	// Also write to zinfo writer to track compression spans
	if _, err := w.gzipZinfoWriter.Write(p[:n]); err != nil {
		return n, fmt.Errorf("failed to write to zinfo: %w", err)
	}

	return n, nil
}

func (w *sociGzipWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true

	// Close gzip writer
	if err := w.gzipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Close zinfo writer
	if err := w.gzipZinfoWriter.Close(); err != nil {
		return fmt.Errorf("failed to close zinfo writer: %w", err)
	}

	// Get the compressed layer digest
	layerDigest := digest.NewDigestFromBytes(digest.SHA256, w.layerHasher.Sum(nil))

	// Build the ztoc
	ztocData, err := w.buildZtoc()
	if err != nil {
		return fmt.Errorf("failed to build ztoc: %w", err)
	}

	// Calculate ztoc digest
	ztocDigest := sha256.Sum256(ztocData)

	w.ztocInfo = &ZtocInfo{
		Digest:      "sha256:" + hex.EncodeToString(ztocDigest[:]),
		Size:        int64(len(ztocData)),
		Bytes:       ztocData,
		LayerDigest: layerDigest.String(),
	}

	return nil
}

func (w *sociGzipWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	return w.gzipWriter.Flush()
}

func (w *sociGzipWriter) buildZtoc() ([]byte, error) {
	// Get TOC from builder
	toc := w.tocBuilder.TOC()

	// Get zinfo data
	zinfoData := w.gzipZinfoWriter.ZInfo()

	// Create ztoc
	ztocBuilder := ztoc.NewBuilder()
	ztocBuilder.SetVersion(ztoc.Version)
	ztocBuilder.SetTOC(toc)
	ztocBuilder.SetCompression(ztoc.CompressionGzip)

	// Set zinfo with span info
	ztocBuilder.SetZinfo(zinfoData)

	// Build and return ztoc bytes
	return ztocBuilder.Bytes()
}

func (w *sociGzipWriter) AppendTarHeader(hdr *tar.Header) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	// Record the entry in TOC
	if err := w.tocBuilder.AppendTarHeader(hdr, w.tarOffset); err != nil {
		return fmt.Errorf("failed to append tar header to TOC: %w", err)
	}

	// Update tar offset
	w.tarOffset += 512 // tar header size
	if hdr.Size > 0 {
		w.tarOffset += hdr.Size
		// Round up to 512 byte blocks
		if remainder := hdr.Size % 512; remainder != 0 {
			w.tarOffset += 512 - remainder
		}
	}

	return nil
}

func (w *sociGzipWriter) ZtocInfo() *ZtocInfo {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ztocInfo
}

// SOCIGzipWriter wraps sociGzipWriter to implement the compress.Writer interface
type SOCIGzipWriter struct {
	*sociGzipWriter
}

func NewSOCIGzipWriter(w io.Writer, opts SOCIOptions) (*SOCIGzipWriter, error) {
	sgw, err := newSOCIGzipWriter(w, opts)
	if err != nil {
		return nil, err
	}
	return &SOCIGzipWriter{sociGzipWriter: sgw}, nil
}

// TarAppender returns a tar appender that tracks tar entries for SOCI
func (w *SOCIGzipWriter) TarAppender() *SOCITarAppender {
	return &SOCITarAppender{writer: w.sociGzipWriter}
}

// SOCITarAppender wraps tar operations to track entries for SOCI ztoc
type SOCITarAppender struct {
	writer *sociGzipWriter
	tw     *tar.Writer
}

func (ta *SOCITarAppender) WriteHeader(hdr *tar.Header) error {
	if ta.tw == nil {
		ta.tw = tar.NewWriter(ta.writer)
	}

	// Record header in SOCI TOC
	if err := ta.writer.AppendTarHeader(hdr); err != nil {
		return err
	}

	// Write header to tar
	return ta.tw.WriteHeader(hdr)
}

func (ta *SOCITarAppender) Write(p []byte) (n int, err error) {
	if ta.tw == nil {
		return 0, fmt.Errorf("tar writer not initialized")
	}
	return ta.tw.Write(p)
}

func (ta *SOCITarAppender) Close() error {
	if ta.tw == nil {
		return nil
	}
	return ta.tw.Close()
}

// Factory maker for SOCI gzip compression
type SOCIGzipMaker struct {
	Options SOCIOptions
}

func (m SOCIGzipMaker) NewWriter(w io.Writer) *SOCIGzipWriter {
	writer, _ := NewSOCIGzipWriter(w, m.Options)
	return writer
}

func (m SOCIGzipMaker) NewWriterLevel(w io.Writer, level int) (*SOCIGzipWriter, error) {
	// For now, ignore level and use default gzip compression
	return NewSOCIGzipWriter(w, m.Options)
}

func (m SOCIGzipMaker) Name() string {
	return "soci-gzip"
}

// Error types for SOCI-specific failures
type SOCIZstdError struct{}

func (e SOCIZstdError) Error() string {
	return "SOCI currently supports gzip layers only; zstd is not supported by major runtimes/snapshotters"
}

// ValidateSOCICompression checks if the compression type is compatible with SOCI
func ValidateSOCICompression(compression string, sociEnabled bool, requireGzip bool) error {
	if !sociEnabled {
		return nil
	}

	if compression != "gzip" {
		if requireGzip {
			return SOCIZstdError{}
		}
		// Log warning but continue
		fmt.Printf("Warning: SOCI is enabled but compression is %s (not gzip). SOCI will be skipped.\n", compression)
	}

	return nil
}
