package main

import (
	"errors"
	"strings"
)

type blobStores []string

func (s *blobStores) String() string {
	if s == nil || len(*s) == 0 {
		return ""
	}
	return strings.Join(*s, ", ")
}

func (s *blobStores) Set(value string) error {
	switch strings.ToLower(value) {
	case "s3", "reapi", "upstream":
		// Valid values, do nothing.
	default:
		return errors.New("invalid blob store type: " + value)
	}
	*s = append(*s, value)
	return nil
}
