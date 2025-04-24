package api

import (
	"archive/tar"
	"io"
	"io/fs"
	"iter"
)

type FileType struct{ inner string }

func (f FileType) String() string {
	return f.inner
}

var (
	RegularFile = FileType{"f"}
	Directory   = FileType{"d"}
)

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
	Store(r io.Reader) ([]byte, int64, error)
	StoreKnownHashAndSize(r io.Reader, hash []byte, size int64) error
	StoreTree(fsys fs.FS) ([]byte, error)
	StoreTreeKnownHash(fsys fs.FS, hash []byte) error
}

type CASStateSupplier interface {
	BlobHashes() iter.Seq[[]byte]
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
