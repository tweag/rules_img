package manifest

import (
	"fmt"
	"os"
	"strings"
)

type fileList []string

func (l *fileList) String() string {
	return strings.Join(*l, ", ")
}

func (l *fileList) Set(value string) error {
	if _, err := os.Stat(value); err != nil {
		return fmt.Errorf("file %s does not exist: %w", value, err)
	}
	*l = append(*l, value)
	return nil
}

type stringList []string

func (l *stringList) String() string {
	return strings.Join(*l, ", ")
}

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

type stringMap map[string]string

func (m *stringMap) String() string {
	var parts []string
	for k, v := range *m {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, ", ")
}

func (m *stringMap) Set(value string) error {
	if *m == nil {
		*m = make(map[string]string)
	}
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid key=value format: %s", value)
	}
	(*m)[parts[0]] = parts[1]
	return nil
}
