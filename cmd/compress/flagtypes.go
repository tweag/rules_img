package compress

import (
	"fmt"
	"strings"
)

// annotationsFlag implements flag.Value for key-value pairs
type annotationsFlag map[string]string

func (a annotationsFlag) String() string {
	var pairs []string
	for k, v := range a {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(pairs, ",")
}

func (a annotationsFlag) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("annotation must be in format key=value, got: %s", value)
	}
	key := strings.TrimSpace(parts[0])
	val := strings.TrimSpace(parts[1])
	if key == "" {
		return fmt.Errorf("annotation key cannot be empty")
	}
	a[key] = val
	return nil
}
