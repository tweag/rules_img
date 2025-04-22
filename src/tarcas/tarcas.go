package tarcas

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"hash"
	"hash/maphash"
	"io"
	"iter"
)

type CAS[HM hashHelper] struct {
	buf           bytes.Buffer
	currentFile   *tar.Header
	deferredFiles []*tar.Header
	tarFile       *tar.Writer
	hashOrder     [][]byte
	storedHashes  map[uint64]struct{}
	seed          maphash.Seed
	closed        bool
	options
}

func New[HM hashHelper](w io.Writer, opts ...Option) *CAS[HM] {
	options := options{
		appendable:                true,
		structure:                 CASFirst,
		writeHeaderCallbackFilter: WriteHeaderCallbackFilterDefault,
	}
	for _, opt := range opts {
		opt.apply(&options)
	}

	return &CAS[HM]{
		tarFile:      tar.NewWriter(w),
		hashOrder:    [][]byte{},
		storedHashes: make(map[uint64]struct{}),
		seed:         maphash.MakeSeed(),
		options:      options,
	}
}

func (c *CAS[HM]) Import(hashes iter.Seq[[]byte]) {
	for hash := range hashes {
		mapKey := maphash.Bytes(c.seed, hash)
		if _, exists := c.storedHashes[mapKey]; !exists {
			c.storedHashes[mapKey] = struct{}{}
			c.hashOrder = append(c.hashOrder, hash)
		}
	}
}

func (c *CAS[HM]) Export() [][]byte {
	return c.hashOrder
}

// Close closes the tar archive by flushing the padding, and optionally writing the footer.
// If the current file (from a prior call to Writer.WriteHeader) is not fully written,
// then this returns an error.
func (c *CAS[HM]) Close() error {
	if c.closed {
		return nil
	}

	if c.currentFile != nil {
		return fmt.Errorf("current file is not fully written")
	}
	c.closed = true
	for _, hdr := range c.deferredFiles {
		if err := c.writeHeaderOrDefer(hdr); err != nil {
			return fmt.Errorf("error writing deferred header: %w", err)
		}
	}

	if err := c.tarFile.Flush(); err != nil {
		return err
	}
	if c.options.appendable {
		// Appendable tar files do not have a footer.
		// Calling Close on the real tar.Writer would write a footer,
		// so we skip that.
		return nil
	}
	return c.tarFile.Close()
}

func (c *CAS[HM]) Flush() error {
	return c.tarFile.Flush()
}

func (c *CAS[HM]) Write(b []byte) (int, error) {
	if c.currentFile == nil {
		return 0, fmt.Errorf("no current file to write to")
	}
	if c.currentFile.Typeflag != tar.TypeReg {
		return 0, fmt.Errorf("current file is not a regular file")
	}

	if c.buf.Len() >= int(c.currentFile.Size) && len(b) > 0 {
		return 0, tar.ErrWriteTooLong
	}
	if len(b) > int(c.currentFile.Size)-c.buf.Len() {
		return 0, tar.ErrWriteTooLong
	}

	// Write the data to the buffer
	n, err := c.buf.Write(b)
	if err != nil {
		return n, err
	}
	if c.buf.Len() < int(c.currentFile.Size) {
		// Not yet finished writing the current file
		return n, nil
	}

	// Whole file is buffered.
	// Commit the CAS object, followed by a hardlink.
	defer func() {
		c.currentFile = nil
		c.buf.Reset()
	}()

	hash, sz, err := c.Store(&c.buf)
	if err != nil {
		return n, err
	}
	if sz != int64(c.currentFile.Size) {
		return n, fmt.Errorf("size mismatch when storing CAS object in tar: expected %d, wrote %d", c.currentFile.Size, sz)
	}
	contentName := fmt.Sprintf("cas/%x", hash)
	if contentName == c.currentFile.Name {
		// If we were writing to the CAS object itself,
		// we don't need to write a hardlink.
		return n, nil
	}
	header := &tar.Header{
		Typeflag: tar.TypeLink,
		Name:     c.currentFile.Name,
		Linkname: contentName,
	}
	return n, c.writeHeaderOrDefer(header)
}

func (c *CAS[HM]) WriteHeader(hdr *tar.Header) error {
	if c.currentFile != nil {
		return errors.New("current file is not fully written")
	}

	if hdr.Typeflag == tar.TypeReg {
		// Regular file
		c.currentFile = hdr
		return nil
	}

	return c.writeHeaderOrDefer(hdr)
}

func (c *CAS[HM]) Store(r io.Reader) ([]byte, int64, error) {
	if c.currentFile != nil && c.currentFile.Size != int64(c.buf.Len()) {
		return nil, 0, fmt.Errorf("current file is not fully written")
	}

	var helper HM
	var buf bytes.Buffer
	h := helper.New()
	n, err := io.Copy(io.MultiWriter(h, &buf), r)
	if err != nil {
		return nil, n, err
	}
	hash := h.Sum(nil)
	return hash, n, c.StoreKnownHashAndSize(&buf, hash, n)
}

func (c *CAS[HM]) StoreKnownHashAndSize(r io.Reader, hash []byte, size int64) error {
	if c.currentFile != nil && c.currentFile.Size != int64(c.buf.Len()) {
		return fmt.Errorf("current file is not fully written")
	}

	mapKey := maphash.Bytes(c.seed, hash)
	if _, exists := c.storedHashes[mapKey]; exists {
		return nil
	}

	contentName := fmt.Sprintf("cas/%x", hash)
	header := &tar.Header{
		Name: contentName,
		Size: size,
		Mode: 0o555,
	}
	if err := c.tarFile.WriteHeader(header); err != nil {
		return err
	}

	n, err := io.Copy(c.tarFile, r)
	if err != nil {
		return err
	}
	if n != size {
		return fmt.Errorf("size mismatch when storing CAS object in tar: expected %d, wrote %d", size, n)
	}

	c.storedHashes[mapKey] = struct{}{}
	c.hashOrder = append(c.hashOrder, hash)

	return nil
}

func (c *CAS[HM]) writeHeaderOrDefer(hdr *tar.Header) error {
	if hdr.Typeflag != tar.TypeReg && c.structure == CASFirst && !c.closed {
		// Defer writing the header for non-regular files
		// until Close() is called.
		c.deferredFiles = append(c.deferredFiles, hdr)
		return nil
	}

	if c.writeHeaderCallback != nil && callbackModeFromTarType(hdr)&c.writeHeaderCallbackFilter != 0 {
		if err := c.writeHeaderCallback(hdr); err != nil {
			return fmt.Errorf("WriteHeader callback error: %w", err)
		}
	}

	if hdr.Typeflag != tar.TypeReg && c.structure == CASOnly {
		// Skip writing the header for non-regular files
		// if the structure should only contain regular files (CAS objects).
		return nil
	}

	// We are either writing a regular files (CAS object)
	// Or are in intertwined mode (CAS and non-CAS objects are mixed together as they are written)
	// Or we are in CASFirst mode and we are about to close the tar (so we need to write the deferred files)
	return c.tarFile.WriteHeader(hdr)
}

func callbackModeFromTarType(hdr *tar.Header) WriteHeaderCallbackFilter {
	switch hdr.Typeflag {
	case tar.TypeReg:
		return WriteHeaderCallbackRegular
	case tar.TypeDir:
		return WriteHeaderCallbackDir
	case tar.TypeLink:
		return WriteHeaderCallbackLink
	case tar.TypeSymlink:
		return WriteHeaderCallbackSymlink
	}
	return 0
}

type hashHelper interface {
	New() hash.Hash
}
