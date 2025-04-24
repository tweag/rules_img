package tarcas

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"hash"
	"hash/maphash"
	"io"
	"io/fs"
	"iter"
	"path"

	"github.com/malt3/rules_img/src/api"
	"github.com/malt3/rules_img/src/tree/merkle"
)

type CAS[HM hashHelper] struct {
	buf           bytes.Buffer
	currentFile   *tar.Header
	deferredFiles []*tar.Header
	tarFile       *tar.Writer
	hashOrder     [][]byte
	treeOrder     [][]byte
	storedHashes  map[uint64]struct{}
	storedTrees   map[uint64]struct{}
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
		treeOrder:    [][]byte{},
		storedHashes: make(map[uint64]struct{}),
		storedTrees:  make(map[uint64]struct{}),
		seed:         maphash.MakeSeed(),
		options:      options,
	}
}

func (c *CAS[HM]) Import(from api.CASStateSupplier) {
	for hash := range from.BlobHashes() {
		mapKey := maphash.Bytes(c.seed, hash)
		c.storedHashes[mapKey] = struct{}{}
	}
	for hash := range from.TreeHashes() {
		mapKey := maphash.Bytes(c.seed, hash)
		c.storedTrees[mapKey] = struct{}{}
	}
}

func (c *CAS[HM]) Export(to api.CASStateExporter) error {
	return to.Export(&exporterState{
		hashOrder: c.hashOrder,
		treeOrder: c.treeOrder,
	})
}

// Close closes the tar archive by flushing the padding, and optionally writing the footer.
// If the current file (from a prior call to Writer.WriteHeader) is not fully written,
// then this returns an error.
func (c *CAS[HM]) Close() error {
	if c.closed {
		return nil
	}

	if c.currentFile != nil {
		return fmt.Errorf("current file is not fully written (%d/%d bytes)", c.buf.Len(), c.currentFile.Size)
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
		if len(b) == 0 {
			return 0, nil
		}
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
	size := c.currentFile.Size
	name := c.currentFile.Name
	c.currentFile = nil
	defer func() {
		c.buf.Reset()
	}()

	hash, storeSize, err := c.Store(&c.buf)
	if err != nil {
		return n, err
	}
	if storeSize != size {
		return n, fmt.Errorf("size mismatch when storing CAS object in tar: expected %d, wrote %d", c.currentFile.Size, storeSize)
	}
	contentName := fmt.Sprintf(".cas/blob/%x", hash)
	if contentName == name {
		// If we were writing to the CAS object itself,
		// we don't need to write a hardlink.
		return n, nil
	}
	header := &tar.Header{
		Typeflag: tar.TypeLink,
		Name:     name,
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
		// TODO: Check if the header contains any non-standard
		// metadata. If so, we cannot use it as a CAS object.
		// In those cases, we should either fail, or
		// write the full file to the original location.
		c.currentFile = hdr
		if hdr.Size == 0 {
			// try to commit empty files immediately
			// since they might never be written by io.Copy
			_, err := c.Write(nil)
			return err
		}
		return nil
	}

	return c.writeHeaderOrDefer(hdr)
}

func (c *CAS[HM]) Store(r io.Reader) ([]byte, int64, error) {
	if c.currentFile != nil && c.currentFile.Size != int64(c.buf.Len()) {
		return nil, 0, fmt.Errorf("current file is not fully written (%d/%d bytes)", c.buf.Len(), c.currentFile.Size)
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
		return fmt.Errorf("current file is not fully written (%d/%d bytes)", c.buf.Len(), c.currentFile.Size)
	}

	mapKey := maphash.Bytes(c.seed, hash)
	if _, exists := c.storedHashes[mapKey]; exists {
		return nil
	}

	contentName := fmt.Sprintf(".cas/blob/%x", hash)
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

func (c *CAS[HM]) StoreTree(fsys fs.FS) ([]byte, error) {
	var hashMaker HM
	treeHasher := merkle.NewTreeHasher(fsys, hashMaker.New)
	rootHash, err := treeHasher.Build()
	if err != nil {
		return nil, fmt.Errorf("calculating tree hash before storing tree artifact in tar: %w", err)
	}
	return rootHash, c.StoreTreeKnownHash(fsys, rootHash)
}

func (c *CAS[HM]) StoreTreeKnownHash(fsys fs.FS, hash []byte) error {
	// Every regular file in the tree is a CAS object, so we need to store it,
	// along with a hardlink to the CAS object.
	// For now, we don't support any special metadata for tree artifacts and disallow empty directories,
	// so we can get away with storing a single directory entry (for the root directory of the tree).
	if c.currentFile != nil && c.currentFile.Size != int64(c.buf.Len()) {
		return fmt.Errorf("current file is not fully written (%d/%d bytes)", c.buf.Len(), c.currentFile.Size)
	}

	mapKey := maphash.Bytes(c.seed, hash)
	if _, exists := c.storedTrees[mapKey]; exists {
		return nil
	}

	treeBase := fmt.Sprintf(".cas/tree/%x", hash)
	header := &tar.Header{
		Typeflag: tar.TypeDir,
		Name:     treeBase,
		Mode:     0o555,
	}
	if err := c.tarFile.WriteHeader(header); err != nil {
		return err
	}

	// Store the tree children in the tar file.
	if err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking directory %s: %w", p, err)
		}
		if !d.Type().IsRegular() {
			// Skip non-regular files
			return nil
		}
		f, err := fsys.Open(p)
		if err != nil {
			return fmt.Errorf("opening file %s: %w", p, err)
		}
		defer f.Close()
		blobHash, _, err := c.Store(f)
		if err != nil {
			return fmt.Errorf("storing file %s: %w", p, err)
		}

		header := &tar.Header{
			Typeflag: tar.TypeLink,
			Name:     path.Join(treeBase, p),
			Linkname: fmt.Sprintf(".cas/blob/%x", blobHash),
		}
		if err := c.tarFile.WriteHeader(header); err != nil {
			return fmt.Errorf("writing link for %s: %w", p, err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("storing tree artifact %x in tar: %w", hash, err)
	}

	c.storedTrees[mapKey] = struct{}{}
	c.treeOrder = append(c.treeOrder, hash)
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

type exporterState struct {
	hashOrder [][]byte
	treeOrder [][]byte
}

func (e *exporterState) BlobHashes() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for _, hash := range e.hashOrder {
			if !yield(hash) {
				return
			}
		}
	}
}

func (e *exporterState) TreeHashes() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for _, hash := range e.treeOrder {
			if !yield(hash) {
				return
			}
		}
	}
}
