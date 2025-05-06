package api

import (
	"archive/tar"
	"io"
	"io/fs"
	"iter"
)

type (
	CompressionAlgorithm string
	HashAlgorithm        string
	LayerFormat          string
)

const (
	// Compression algorithms
	Uncompressed CompressionAlgorithm = "uncompressed"
	Gzip         CompressionAlgorithm = "gzip"
	// TODO: Zstd CompressionAlgorithm = "zstd"

	// Hash algorithms
	SHA256 HashAlgorithm = "sha256"

	// Layer formats
	TarLayer     = "application/vnd.oci.image.layer.v1.tar"
	TarGzipLayer = "application/vnd.oci.image.layer.v1.tar+gzip"
	// TarZstdLayer = "application/vnd.oci.image.layer.v1.tar+zstd"
)

func (c LayerFormat) CompressionAlgorithm() CompressionAlgorithm {
	switch c {
	case TarLayer:
		return Uncompressed
	case TarGzipLayer:
		return Gzip
	// case TarZstdLayer:
	// 	return Zstd
	default:
		return ""
	}
}

type FileType struct{ inner string }

func (f FileType) String() string {
	return f.inner
}

var (
	RegularFile = FileType{"f"}
	Directory   = FileType{"d"}
)

type LayerMetadata struct {
	DiffID    string `json:"diff_id"`
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type AppenderState struct {
	// Magic is an identifier for the format of the state.
	Magic string `json:"magic"`
	// OuterHashState is the inner state of the hash for the compressed data.
	// Used to resume the hash function for appending.
	OuterHashState []byte `json:"outer_hash_state"`
	// OuterHash is the final hash for the compressed data.
	// Cannot be used for resuming, but is the actual hash.
	OuterHash []byte `json:"outer_hash"`
	// ContentHashState is the state of the hash for the inner, uncompressed data.
	ContentHashState []byte `json:"content_hash_state"`
	// ContentHash is the final hash for the inner, uncompressed data.
	ContentHash []byte `json:"content_hash"`
	// CompressedSize is the compressed size of the blob.
	CompressedSize int64 `json:"compressed_size"`
	// UncompressedSize is the uncompressed size of the blob.
	UncompressedSize int64 `json:"uncompressed_size"`
}

type Appender interface {
	io.Writer
	Finalize() (AppenderState, error)
}

type CAS interface {
	Import(CASStateSupplier)
	Export(CASStateExporter) error
	Store(r io.Reader) (linkPath string, blobHash []byte, blobSize int64, err error)
	StoreKnownHashAndSize(r io.Reader, blobHash []byte, size int64) (linkPath string, err error)
	StoreNode(r io.Reader, hdr *tar.Header) (linkPath string, blobHash []byte, size int64, err error)
	StoreNodeKnownHash(r io.Reader, hdr *tar.Header, blobHash []byte) (linkPath string, err error)
	StoreTree(fsys fs.FS) (linkPath string, err error)
	StoreTreeKnownHash(fsys fs.FS, treeHash []byte) (linkPath string, err error)
}

type CASStateSupplier interface {
	// Blobs are files without any metadata.
	// The hash is the hash of the file contents.
	BlobHashes() iter.Seq[[]byte]
	// Nodes are inodes with metadata.
	// The hash includes any metadata,
	// as well as the file contents.
	NodeHashes() iter.Seq[[]byte]
	// Trees are made up of blobs
	// with paths in the tree.
	TreeHashes() iter.Seq[[]byte]
}

type CASStateExporter interface {
	Export(CASStateSupplier) error
}

type TarWriter interface {
	Close() error
	Flush() error
	Write(b []byte) (int, error)
	WriteHeader(hdr *tar.Header) error
}

type TarCAS interface {
	CAS
	TarWriter
}
