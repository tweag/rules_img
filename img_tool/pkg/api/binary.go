package api

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

func appendByteSlice(b []byte, slice []byte) []byte {
	if len(slice) > 255 {
		// this should never happen (due to real hash functions having a small, fixed size state)
		panic("slice field too long")
	}
	b = append(b, byte(len(slice)))
	b = append(b, slice...)
	return b
}

func consumeByteSlice(b []byte) ([]byte, []byte, error) {
	if len(b) < 1 {
		return nil, nil, errors.New("too short")
	}
	sliceLen := int(b[0])
	b = b[1:]
	if len(b) < sliceLen {
		return nil, nil, errors.New("too short")
	}
	slice := b[:sliceLen]
	b = b[sliceLen:]
	return slice, b, nil
}

func consumeMagic(b []byte) (string, []byte, error) {
	// read the magic string (ends with a null byte)
	magicEnd := 0
	for i := 0; i < len(b); i++ {
		if b[i] == 0 {
			magicEnd = i
			break
		}
	}
	if magicEnd == 0 {
		return "", nil, errors.New("invalid magic")
	}
	magic := string(b[:magicEnd])
	b = b[magicEnd+1:]
	return magic, b, nil
}

func (s *AppenderState) MarshalBinary() ([]byte, error) {
	return s.AppendBinary(nil)
}

func (s *AppenderState) AppendBinary(b []byte) ([]byte, error) {
	if len(s.OuterHashState) > 255 || len(s.OuterHash) > 255 || len(s.ContentHashState) > 255 || len(s.ContentHash) > 255 {
		// this should never happen (due to real hash functions having a small, fixed size state)
		return nil, errors.New("encoding AppenderState: field too long")
	}

	// start with the magic string
	b = append(b, s.Magic...)
	b = append(b, []byte{0}...)

	b = appendByteSlice(b, s.OuterHashState)
	b = appendByteSlice(b, s.OuterHash)
	b = appendByteSlice(b, s.ContentHashState)
	b = appendByteSlice(b, s.ContentHash)
	b = binary.BigEndian.AppendUint64(b, uint64(s.CompressedSize))
	b = binary.BigEndian.AppendUint64(b, uint64(s.UncompressedSize))

	return b, nil
}

func (s *AppenderState) UnmarshalBinary(b []byte) error {
	if len(b) < len("imgv1+compressed+") {
		return errors.New("decoding AppenderState: too short")
	}
	magic, b, err := consumeMagic(b)
	if err != nil {
		return fmt.Errorf("decoding AppenderState: %w", err)
	}
	s.Magic = magic
	if !strings.HasPrefix(s.Magic, "imgv1+compressed+") {
		return errors.New("decoding AppenderState: invalid magic")
	}

	// read the outer hash
	s.OuterHashState, b, err = consumeByteSlice(b)
	if err != nil {
		return fmt.Errorf("decoding AppenderState: %w", err)
	}
	s.OuterHash, b, err = consumeByteSlice(b)
	if err != nil {
		return fmt.Errorf("decoding AppenderState: %w", err)
	}
	// read the content hash
	s.ContentHashState, b, err = consumeByteSlice(b)
	if err != nil {
		return fmt.Errorf("decoding AppenderState: %w", err)
	}
	s.ContentHash, b, err = consumeByteSlice(b)
	if err != nil {
		return fmt.Errorf("decoding AppenderState: %w", err)
	}
	// read the sizes
	if len(b) < 16 {
		return errors.New("decoding AppenderState: too short")
	}
	s.CompressedSize = int64(binary.BigEndian.Uint64(b[:8]))
	s.UncompressedSize = int64(binary.BigEndian.Uint64(b[8:16]))
	b = b[16:]
	if len(b) != 0 {
		return errors.New("decoding AppenderState: extra data")
	}
	return nil
}
