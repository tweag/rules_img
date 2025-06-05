package layerpresence

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/tweag/rules_img/pkg/api"
)

type layerMetadata map[int]api.LayerMetadata

func (l *layerMetadata) String() string {
	if l == nil || *l == nil {
		return ""
	}
	var b strings.Builder
	keys := slices.Collect(maps.Keys(*l))
	slices.Sort(keys)
	for i, key := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%d=%s", key, (*l)[key].Digest)
	}
	return b.String()
}

func (l *layerMetadata) Set(value string) error {
	if *l == nil {
		*l = make(layerMetadata)
	}
	kv := strings.SplitN(value, "=", 2)
	if len(kv) != 2 {
		return fmt.Errorf("invalid argument %s: expected key-value pair separated by equals", value)
	}
	index, err := strconv.Atoi(kv[0])
	if err != nil {
		return err
	}
	rawMetadata, err := os.ReadFile(kv[1])
	if err != nil {
		return err
	}
	var metadata api.LayerMetadata
	if err := json.Unmarshal(rawMetadata, &metadata); err != nil {
		return err
	}
	(*l)[index] = metadata
	return nil
}

type validationOutputs []io.WriteCloser

func (o *validationOutputs) String() string {
	var b strings.Builder
	for i, w := range *o {
		if i > 0 {
			b.WriteString(", ")
		}
		if w == os.Stdout {
			b.WriteString("stdout")
			continue
		} else if w == os.Stderr {
			b.WriteString("stderr")
			continue
		}
		switch writer := w.(type) {
		case *os.File:
			b.WriteString(writer.Name())
		default:
			b.WriteString("unknown")
		}
	}
	return b.String()
}

func (o *validationOutputs) Set(value string) error {
	if value == "-" {
		*o = append(*o, os.Stdout)
		return nil
	}
	f, err := os.Create(value)
	if err != nil {
		return err
	}
	*o = append(*o, f)
	return nil
}
