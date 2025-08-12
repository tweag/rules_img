package docker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Load pipes the tar stream to docker load
func Load(tarReader io.Reader) error {
	cmd := exec.Command("docker", "load")
	cmd.Stdin = tarReader
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker load failed: %w", err)
	}

	return nil
}

// NormalizeTag normalizes a tag for Docker
func NormalizeTag(tag string) string {
	if tag == "" {
		return ""
	}

	// Docker load expects the full image reference
	// The normalization happens in the Load function in pkg/load
	return tag
}
