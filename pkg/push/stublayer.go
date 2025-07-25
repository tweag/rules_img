package push

import (
	"errors"
	"io"
)

type stubBlob struct{}

func (s *stubBlob) Compressed() (io.ReadCloser, error) {
	return nil, errors.New("This layer is missing from the registry, but should be present. There is no local source for the data, so it cannot be pushed.")
}
