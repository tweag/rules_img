package docker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Load pipes the tar stream to docker load
func Load(tarReader io.Reader) error {
	// Use LOADER environment variable if set, otherwise default to "docker"
	dockerBinary := os.Getenv("LOADER")
	if dockerBinary == "" {
		dockerBinary = "docker"
	}

	if _, err := exec.LookPath(dockerBinary); err != nil {
		return fmt.Errorf("%s not found in PATH: %w", dockerBinary, err)
	}

	fmt.Printf("Loading image using %s load...\n", dockerBinary)

	cmd := exec.Command(dockerBinary, "load")
	cmd.Stdin = tarReader
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s load failed: %w", dockerBinary, err)
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
