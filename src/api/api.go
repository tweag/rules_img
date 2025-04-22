package api

import (
	"archive/tar"
	"io"
	"iter"
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
	Import(hashes iter.Seq[[]byte])
	Export() [][]byte
	Store(r io.Reader) ([]byte, int64, error)
	StoreKnownHashAndSize(r io.Reader, hash []byte, size int64) error
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
