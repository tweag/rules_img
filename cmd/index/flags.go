package index

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type manifestDescriptors []specsv1.Descriptor

func (d *manifestDescriptors) String() string {
	if d == nil {
		return ""
	}
	var sb strings.Builder
	for i, d := range *d {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(string(d.Digest))
	}
	return sb.String()
}

func (d *manifestDescriptors) Set(value string) error {
	rawDescriptor, err := os.ReadFile(value)
	if err != nil {
		return err
	}
	var descriptor specsv1.Descriptor
	if err := json.Unmarshal(rawDescriptor, &descriptor); err != nil {
		return err
	}
	*d = append(*d, descriptor)
	return nil
}

type annotations map[string]string

func (a *annotations) String() string {
	if a == nil {
		return ""
	}
	var sb strings.Builder
	keys := slices.Collect(maps.Keys(*a))
	slices.Sort(keys)
	for i, key := range keys {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(key)
		sb.WriteString("=")
		sb.WriteString((*a)[key])
	}
	return sb.String()
}

func (a *annotations) Set(value string) error {
	if *a == nil {
		*a = make(annotations)
	}
	kv := strings.SplitN(value, "=", 2)
	if len(kv) != 2 {
		return fmt.Errorf("expected annoation as key-value pair separated by equals, but got %s", kv)
	}
	(*a)[kv[0]] = kv[1]
	return nil
}
