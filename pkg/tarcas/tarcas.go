package tarcas

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"iter"
	"path"
	"strings"

	"github.com/tweag/rules_img/pkg/api"
	"github.com/tweag/rules_img/pkg/tree/merkle"
)

type CAS[HM hashHelper] struct {
	buf           bytes.Buffer
	currentFile   *tar.Header
	deferredFiles []*tar.Header
	tarFile       *tar.Writer
	hashOrder     [][]byte
	nodeOrder     [][]byte
	treeOrder     [][]byte
	storedHashes  map[string]struct{}
	storedNodes   map[string]struct{}
	storedTrees   map[string]struct{}
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
		nodeOrder:    [][]byte{},
		treeOrder:    [][]byte{},
		storedHashes: make(map[string]struct{}),
		storedNodes:  make(map[string]struct{}),
		storedTrees:  make(map[string]struct{}),
		options:      options,
	}
}

func (c *CAS[HM]) Import(from api.CASStateSupplier) error {
	for hash, err := range from.BlobHashes() {
		if err != nil {
			return err
		}
		c.storedHashes[string(hash)] = struct{}{}
	}
	for hash, err := range from.NodeHashes() {
		if err != nil {
			return err
		}
		c.storedNodes[string(hash)] = struct{}{}
	}
	for hash, err := range from.TreeHashes() {
		if err != nil {
			return err
		}
		c.storedTrees[string(hash)] = struct{}{}
	}
	return nil
}

func (c *CAS[HM]) Export(to api.CASStateExporter) error {
	return to.Export(&exporterState{
		hashOrder: c.hashOrder,
		nodeOrder: c.nodeOrder,
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
	th := c.currentFile
	c.currentFile = nil
	defer func() {
		c.buf.Reset()
	}()

	var helper HM
	blobHasher := helper.New()
	blobHasher.Write(c.buf.Bytes())
	blobHash := blobHasher.Sum(nil)

	var linkPath string
	if isBlobTarHeader(th) {
		// Try to store a blob without special metadata.
		// This only works for regular files
		// with hardcoded rwxr-xr-x permissions.
		var err error
		linkPath, err = c.StoreKnownHashAndSize(&c.buf, blobHash, th.Size)
		if err != nil {
			return n, err
		}
	} else {
		// This file has more complex requirements.
		// Store it as a "node" instead, including metadata.
		var err error
		linkPath, err = c.StoreNodeKnownHash(&c.buf, th, blobHash)
		if err != nil {
			return n, err
		}
	}

	if linkPath == th.Name {
		// If we were writing to the CAS object itself,
		// we don't need to write a hardlink.
		return n, nil
	}
	header := cloneTarHeader(th)
	header.Typeflag = tar.TypeLink
	header.Linkname = linkPath
	return n, c.writeHeaderOrDefer(&header)
}

func (c *CAS[HM]) WriteHeader(hdr *tar.Header) error {
	if c.currentFile != nil {
		return errors.New("current file is not fully written")
	}

	if hdr.Typeflag == tar.TypeReg && !strings.HasSuffix(hdr.Name, "/") {
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

func (c *CAS[HM]) Store(r io.Reader) (string, []byte, int64, error) {
	if c.currentFile != nil && c.currentFile.Size != int64(c.buf.Len()) {
		return "", nil, 0, fmt.Errorf("current file is not fully written (%d/%d bytes)", c.buf.Len(), c.currentFile.Size)
	}

	var helper HM
	var buf bytes.Buffer
	h := helper.New()
	n, err := io.Copy(io.MultiWriter(h, &buf), r)
	if err != nil {
		return "", nil, n, err
	}
	hash := h.Sum(nil)
	contentPath, err := c.StoreKnownHashAndSize(&buf, hash, n)
	return contentPath, hash, n, err
}

func (c *CAS[HM]) StoreKnownHashAndSize(r io.Reader, hash []byte, size int64) (string, error) {
	if c.currentFile != nil && c.currentFile.Size != int64(c.buf.Len()) {
		return "", fmt.Errorf("current file is not fully written (%d/%d bytes)", c.buf.Len(), c.currentFile.Size)
	}

	contentName := casPath("blob", hash)

	if _, exists := c.storedHashes[string(hash)]; exists {
		return contentName, nil
	}

	header := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     contentName,
		Size:     size,
		Mode:     0o755,
	}
	if err := c.tarFile.WriteHeader(header); err != nil {
		return "", err
	}

	n, err := io.Copy(c.tarFile, r)
	if err != nil {
		return "", err
	}
	if n != size {
		return "", fmt.Errorf("size mismatch when storing CAS object in tar: expected %d, wrote %d", size, n)
	}

	c.storedHashes[string(hash)] = struct{}{}
	c.hashOrder = append(c.hashOrder, hash)

	return contentName, nil
}

func (c *CAS[HM]) StoreNode(r io.Reader, hdr *tar.Header) (linkPath string, blobHash []byte, size int64, err error) {
	// TODO: cache content hashing in vfs
	var helper HM
	var buf bytes.Buffer
	h := helper.New()
	n, err := io.Copy(io.MultiWriter(h, &buf), r)
	if err != nil {
		return "", nil, n, err
	}
	blobHash = h.Sum(nil)
	linkPath, err = c.StoreNodeKnownHash(&buf, hdr, blobHash)
	return linkPath, blobHash, size, err
}

func (c *CAS[HM]) StoreNodeKnownHash(r io.Reader, hdr *tar.Header, blobHash []byte) (linkPath string, err error) {
	var helper HM
	if c.currentFile != nil && c.currentFile.Size != int64(c.buf.Len()) {
		return "", fmt.Errorf("current file is not fully written (%d/%d bytes)", c.buf.Len(), c.currentFile.Size)
	}

	// nodes are like blobs (regular files with content),
	// but they also have metadata (like permissions, owner, group, mtime, xattrs, etc.)
	// we need to account for that in the hash

	if hdr.Typeflag != tar.TypeReg || strings.HasSuffix(hdr.Name, "/") {
		// only regular files can be stored as nodes
		// other kinds cannot be targets of hardlinks
		return "", fmt.Errorf("invalid node header: %s", hdr.Name)
	}

	// create a normalized version of the header
	recordedTarHeader := cloneTarHeader(hdr)
	// we explicitly leave the name empty for hashing
	// so that files in different locations can hardlink the same
	// CAS entry.
	recordedTarHeader.Name = ""
	normalizeTarHeader(&recordedTarHeader)

	hasher := helper.New()
	hashTarHeader(hasher, recordedTarHeader)
	hasher.Write(blobHash)
	nodeHash := hasher.Sum(nil)

	linkPath = casPath("node", nodeHash)

	if _, exists := c.storedNodes[string(nodeHash)]; exists {
		return linkPath, nil
	}

	recordedTarHeader.Name = linkPath
	if err := c.tarFile.WriteHeader(&recordedTarHeader); err != nil {
		return linkPath, err
	}

	n, err := io.Copy(c.tarFile, r)
	if err != nil {
		return linkPath, err
	}
	if n != recordedTarHeader.Size {
		return linkPath, fmt.Errorf("size mismatch when storing CAS object in tar: expected %d, wrote %d", recordedTarHeader.Size, n)
	}

	c.storedNodes[string(nodeHash)] = struct{}{}
	c.nodeOrder = append(c.nodeOrder, nodeHash)
	return linkPath, nil
}

func (c *CAS[HM]) StoreTree(fsys fs.FS) (linkPath string, err error) {
	var hashMaker HM
	treeHasher := merkle.NewTreeHasher(fsys, hashMaker.New)
	rootHash, err := treeHasher.Build()
	if err != nil {
		return "", fmt.Errorf("calculating tree hash before storing tree artifact in tar: %w", err)
	}
	return c.StoreTreeKnownHash(fsys, rootHash)
}

func (c *CAS[HM]) StoreTreeKnownHash(fsys fs.FS, treeHash []byte) (linkPath string, err error) {
	// Every regular file in the tree is a CAS object, so we need to store it,
	// along with a hardlink to the CAS object.
	// For now, we don't support any special metadata for tree artifacts and disallow empty directories,
	// so we can get away with storing a single directory entry (for the root directory of the tree).
	if c.currentFile != nil && c.currentFile.Size != int64(c.buf.Len()) {
		return "", fmt.Errorf("current file is not fully written (%d/%d bytes)", c.buf.Len(), c.currentFile.Size)
	}

	treeBase := casPath("tree", treeHash)
	if _, exists := c.storedTrees[string(treeHash)]; exists {
		return treeBase, nil
	}

	header := &tar.Header{
		Typeflag: tar.TypeDir,
		Name:     treeBase,
		Mode:     0o755,
	}
	if err := c.tarFile.WriteHeader(header); err != nil {
		return treeBase, err
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
		linkName, _, _, err := c.Store(f)
		if err != nil {
			return fmt.Errorf("storing file %s: %w", p, err)
		}

		header := &tar.Header{
			Typeflag: tar.TypeLink,
			Name:     path.Join(treeBase, p),
			Linkname: linkName,
			Mode:     0o755,
		}
		if err := c.tarFile.WriteHeader(header); err != nil {
			return fmt.Errorf("writing link for %s: %w", p, err)
		}
		return nil
	}); err != nil {
		return treeBase, fmt.Errorf("storing tree artifact %x in tar: %w", treeHash, err)
	}

	c.storedTrees[string(treeHash)] = struct{}{}
	c.treeOrder = append(c.treeOrder, treeHash)
	return treeBase, nil
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

func casPath(blobKind string, hash []byte) string {
	return fmt.Sprintf(".cas/%s/%x", blobKind, hash)
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
	nodeOrder [][]byte
	treeOrder [][]byte
}

func (e *exporterState) BlobHashes() iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for _, hash := range e.hashOrder {
			if !yield(hash, nil) {
				return
			}
		}
	}
}

func (e *exporterState) NodeHashes() iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for _, hash := range e.nodeOrder {
			if !yield(hash, nil) {
				return
			}
		}
	}
}

func (e *exporterState) TreeHashes() iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for _, hash := range e.treeOrder {
			if !yield(hash, nil) {
				return
			}
		}
	}
}
