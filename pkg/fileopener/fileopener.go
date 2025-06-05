package fileopener

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/tweag/rules_img/pkg/api"
)

func CompressionReaderWithFormat(r io.Reader, format api.CompressionAlgorithm) (io.Reader, error) {
	switch format {
	case api.Gzip:
		return gzip.NewReader(r)
	case api.Uncompressed, "tar", "none":
		return r, nil
	default:
		return nil, fmt.Errorf("unsupported compression format: %s", format)
	}
}

func CompressionReader(r underlyingReader) (io.Reader, error) {
	compressionFormat, err := LearnCompressionAlgorithm(r)
	if err != nil {
		return nil, err
	}

	return CompressionReaderWithFormat(r, compressionFormat)
}

func LearnCompressionAlgorithm(r io.ReaderAt) (api.CompressionAlgorithm, error) {
	var startMagic [4]byte
	if _, err := r.ReadAt(startMagic[:], 0); err != nil {
		return "", err
	}
	if bytes.Compare(startMagic[:2], gzipMagic[:]) == 0 {
		return api.Gzip, nil
	}
	// if bytes.Compare(startMagic[:4], zstdMagic[:]) == 0 {
	// 	return api.Zstd, nil
	// }
	return api.Uncompressed, nil
}

func LearnLayerFormat(r io.ReaderAt) (api.LayerFormat, error) {
	compressionFormat, err := LearnCompressionAlgorithm(r)
	if err != nil {
		return "", err
	}

	if compressionFormat != api.Uncompressed {
		switch compressionFormat {
		case api.Gzip:
			return api.TarGzipLayer, nil
		default:
			return "", fmt.Errorf("unsupported compression format: %s", compressionFormat)
		}
	}

	var tarMagic [8]byte
	if _, err := r.ReadAt(tarMagic[:], 257); err != nil {
		return "", err
	}
	if bytes.Compare(tarMagic[:], tarMagicA[:]) == 0 || bytes.Compare(tarMagic[:], tarMagicB[:]) == 0 {
		return api.TarLayer, nil
	}
	return "", fmt.Errorf("unknown file type")
}

type underlyingReader interface {
	io.Reader
	io.ReaderAt
}

var (
	gzipMagic = [2]byte{0x1f, 0x8b}
	zstdMagic = [4]byte{0x28, 0xb5, 0x2f, 0xfd}
	tarMagicA = [8]byte{0x75, 0x73, 0x74, 0x61, 0x72, 0x00, 0x30, 0x30}
	tarMagicB = [8]byte{0x75, 0x73, 0x74, 0x61, 0x72, 0x20, 0x20, 0x00}
)
