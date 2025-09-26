package tarcas

import (
	"bytes"
	"io"
	"os"
	"runtime"
)

const maxMemoryBuffer = 64 * 1024 * 1024 // 64 MiB

type SizeAwareBuffer struct {
	memBuf  *bytes.Buffer
	tmpFile *os.File
	size    int64
	isTemp  bool
}

func NewSizeAwareBuffer(expectedSize int64) *SizeAwareBuffer {
	if expectedSize >= 0 && expectedSize <= maxMemoryBuffer {
		return &SizeAwareBuffer{
			memBuf: &bytes.Buffer{},
			size:   0,
			isTemp: false,
		}
	}

	tmpFile, err := os.CreateTemp("", "tarcas-buffer-*.tmp")
	if err != nil {
		return &SizeAwareBuffer{
			memBuf: &bytes.Buffer{},
			size:   0,
			isTemp: false,
		}
	}

	buf := &SizeAwareBuffer{
		tmpFile: tmpFile,
		size:    0,
		isTemp:  true,
	}

	runtime.SetFinalizer(buf, (*SizeAwareBuffer).cleanup)
	return buf
}

func (b *SizeAwareBuffer) Write(p []byte) (n int, err error) {
	if b.isTemp {
		n, err = b.tmpFile.Write(p)
		b.size += int64(n)
		return n, err
	}

	n, err = b.memBuf.Write(p)
	b.size += int64(n)

	if b.size > maxMemoryBuffer {
		if err2 := b.switchToTempFile(); err2 != nil && err == nil {
			err = err2
		}
	}

	return n, err
}

func (b *SizeAwareBuffer) switchToTempFile() error {
	tmpFile, err := os.CreateTemp("", "tarcas-buffer-*.tmp")
	if err != nil {
		return err
	}

	if _, err := io.Copy(tmpFile, b.memBuf); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return err
	}

	b.tmpFile = tmpFile
	b.memBuf = nil
	b.isTemp = true

	runtime.SetFinalizer(b, (*SizeAwareBuffer).cleanup)
	return nil
}

func (b *SizeAwareBuffer) Bytes() []byte {
	if b.isTemp {
		return nil
	}
	return b.memBuf.Bytes()
}

func (b *SizeAwareBuffer) Reader() (io.ReadSeeker, error) {
	if b.isTemp {
		if _, err := b.tmpFile.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		return b.tmpFile, nil
	}
	return bytes.NewReader(b.memBuf.Bytes()), nil
}

func (b *SizeAwareBuffer) Close() error {
	return b.cleanup()
}

func (b *SizeAwareBuffer) cleanup() error {
	if b.isTemp && b.tmpFile != nil {
		name := b.tmpFile.Name()
		if err := b.tmpFile.Close(); err != nil {
			os.Remove(name)
			return err
		}
		if err := os.Remove(name); err != nil {
			return err
		}
		b.tmpFile = nil
		b.isTemp = false
		runtime.SetFinalizer(b, nil)
	}
	return nil
}
