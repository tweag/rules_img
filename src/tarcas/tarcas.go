package tarcas

import (
	"archive/tar"
	"bytes"
	"fmt"
	"hash"
	"io"
	"iter"
)

type CAS[H comparable, HW hash.Hash, HM hashHelper[H, HW]] struct {
	tarFileContents   *tar.Writer
	tarFileReferences *tar.Writer
	hashOrder         []H
	storedHashes      map[H]struct{}
}

func NewCAS[H comparable, HW hash.Hash, HM hashHelper[H, HW]](contentWriter, referencesWriter io.Writer) CAS[H, HW, HM] {
	return CAS[H, HW, HM]{
		tarFileContents:   tar.NewWriter(contentWriter),
		tarFileReferences: tar.NewWriter(referencesWriter),
		hashOrder:         []H{},
		storedHashes:      make(map[H]struct{}),
	}
}

func (c *CAS[H, HW, HM]) Import(hashes iter.Seq[H]) {
	for hash := range hashes {
		if _, exists := c.storedHashes[hash]; !exists {
			c.storedHashes[hash] = struct{}{}
			c.hashOrder = append(c.hashOrder, hash)
		}
	}
}

func (c *CAS[H, HW, HM]) Export() []H {
	return c.hashOrder
}

func (c *CAS[H, HW, HM]) Close() error {
	return c.tarFileContents.Close()
}

func (c *CAS[H, HW, HM]) Store(name string, r io.Reader) (H, int64, error) {
	var helper HM
	var buf bytes.Buffer
	h := helper.New()
	n, err := io.Copy(io.MultiWriter(h, &buf), r)
	if err != nil {
		return helper.Sum(helper.New()), n, err
	}
	hash := helper.Sum(h)
	return hash, n, c.StoreKnownHashAndSize(name, &buf, hash, n)
}

func (c *CAS[H, HW, HM]) StoreKnownHashAndSize(name string, r io.Reader, hash H, size int64) error {
	if _, exists := c.storedHashes[hash]; exists {
		return nil
	}

	var helper HM
	contentName := fmt.Sprintf("cas/%s", helper.Hex(hash))
	header := &tar.Header{
		Name: contentName,
		Size: size,
		Mode: 0o555,
	}
	if err := c.tarFileContents.WriteHeader(header); err != nil {
		return err
	}

	n, err := io.Copy(c.tarFileContents, r)
	if err != nil {
		return err
	}
	if n != size {
		return fmt.Errorf("size mismatch when storing CAS object in tar: expected %d, wrote %d", size, n)
	}

	c.storedHashes[hash] = struct{}{}
	c.hashOrder = append(c.hashOrder, hash)

	header = &tar.Header{
		Typeflag: tar.TypeLink,
		Name:     name,
		Linkname: contentName,
	}
	if err := c.tarFileReferences.WriteHeader(header); err != nil {
		return err
	}

	return nil
}

type hashHelper[H comparable, HW hash.Hash] interface {
	New() HW
	Sum(HW) H
	Hex(H) string
}
