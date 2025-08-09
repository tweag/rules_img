package ocilayout

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	v1 "github.com/malt3/go-containerregistry/pkg/v1"
)

const OCILayoutVersion = "1.0.0"

type blobMap map[string]string // digest -> source path

func OCILayoutProcess(ctx context.Context, args []string) {
	var manifestPath string
	var indexPath string
	var outputDir string
	var configPath string
	var layerFlags layerMappingFlag
	var manifestPaths stringSliceFlag
	var configPaths stringSliceFlag
	var useSymlinks bool

	flagSet := flag.NewFlagSet("oci-layout", flag.ExitOnError)
	flagSet.Usage = func() {
		fmt.Fprintf(flagSet.Output(), "Assembles an OCI layout directory from manifest/index and layers.\n\n")
		fmt.Fprintf(flagSet.Output(), "Usage: img oci-layout [OPTIONS]\n")
		flagSet.PrintDefaults()
		examples := []string{
			"img oci-layout --manifest manifest.json --config config.json --layer layer1_meta.json=layer1.tar.gz --output oci-layout",
			"img oci-layout --index index.json --manifest-path m1.json --config-path c1.json --layer l1_meta.json=l1.tar.gz --output oci-layout",
		}
		fmt.Fprintf(flagSet.Output(), "\nExamples:\n")
		for _, example := range examples {
			fmt.Fprintf(flagSet.Output(), "  $ %s\n", example)
		}
	}

	flagSet.StringVar(&manifestPath, "manifest", "", "Path to the image manifest (for single manifest)")
	flagSet.StringVar(&indexPath, "index", "", "Path to the image index (for multi-platform)")
	flagSet.StringVar(&configPath, "config", "", "Path to the image config (for single manifest)")
	flagSet.StringVar(&outputDir, "output", "", "Output directory for OCI layout (required)")
	flagSet.Var(&layerFlags, "layer", "Layer mapping in format metadata=blob (can be specified multiple times)")
	flagSet.Var(&manifestPaths, "manifest-path", "Path to manifest file (for index, can be specified multiple times)")
	flagSet.Var(&configPaths, "config-path", "Path to config file (for index, can be specified multiple times)")
	flagSet.BoolVar(&useSymlinks, "symlink", false, "Use symlinks instead of copying files")

	if err := flagSet.Parse(args); err != nil {
		flagSet.Usage()
		os.Exit(1)
	}

	// Validate required flags
	if outputDir == "" {
		fmt.Fprintf(os.Stderr, "Error: --output is required\n")
		flagSet.Usage()
		os.Exit(1)
	}

	var err error
	if indexPath != "" {
		if manifestPath != "" || configPath != "" {
			fmt.Fprintf(os.Stderr, "Error: cannot use --manifest or --config with --index\n")
			os.Exit(1)
		}
		if len(manifestPaths) != len(configPaths) {
			fmt.Fprintf(os.Stderr, "Error: number of --manifest-path must match --config-path\n")
			os.Exit(1)
		}
		if len(manifestPaths) == 0 {
			fmt.Fprintf(os.Stderr, "Error: --index requires at least one --manifest-path and --config-path\n")
			os.Exit(1)
		}
		err = assembleOCILayoutWithIndex(indexPath, outputDir, manifestPaths, configPaths, layerFlags, useSymlinks)
	} else {
		if manifestPath == "" {
			fmt.Fprintf(os.Stderr, "Error: either --manifest or --index is required\n")
			flagSet.Usage()
			os.Exit(1)
		}
		if configPath == "" {
			fmt.Fprintf(os.Stderr, "Error: --config is required when using --manifest\n")
			flagSet.Usage()
			os.Exit(1)
		}
		if len(manifestPaths) > 0 || len(configPaths) > 0 {
			fmt.Fprintf(os.Stderr, "Error: cannot use --manifest-path or --config-path without --index\n")
			os.Exit(1)
		}
		err = assembleOCILayout(manifestPath, configPath, outputDir, layerFlags, useSymlinks)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func assembleOCILayout(manifestPath, configPath, outputDir string, layers layerMappingFlag, useSymlinks bool) error {
	if err := setupOCILayout(outputDir); err != nil {
		return err
	}

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	var manifest v1.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("unmarshaling manifest: %w", err)
	}

	// Build a map of available layers by their digest
	layerBlobsByDigest := make(map[string]string)
	for _, layer := range layers {
		metadataData, err := os.ReadFile(layer.metadata)
		if err != nil {
			return fmt.Errorf("reading layer metadata %s: %w", layer.metadata, err)
		}

		var metadata struct {
			Digest string `json:"digest"`
		}
		if err := json.Unmarshal(metadataData, &metadata); err != nil {
			return fmt.Errorf("unmarshaling layer metadata %s: %w", layer.metadata, err)
		}

		// Extract hex digest from sha256:xxxx format
		digest := strings.TrimPrefix(metadata.Digest, "sha256:")
		layerBlobsByDigest[digest] = layer.blob
	}

	blobs := make(blobMap)
	blobs[manifest.Config.Digest.Hex] = configPath

	// Only add layers that we have blobs for
	for _, layerDesc := range manifest.Layers {
		if blobPath, ok := layerBlobsByDigest[layerDesc.Digest.Hex]; ok {
			blobs[layerDesc.Digest.Hex] = blobPath
		}
	}

	blobsDir := filepath.Join(outputDir, "blobs", "sha256")

	// Copy manifest to blobs directory
	manifestDigest := hashBytes(manifestData)
	blobs[manifestDigest.Hex] = manifestPath

	if err := copyBlobs(blobs, blobsDir, useSymlinks); err != nil {
		return err
	}

	index := v1.IndexManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.index.v1+json",
		Manifests: []v1.Descriptor{
			{
				MediaType: manifest.MediaType,
				Digest:    manifestDigest,
				Size:      int64(len(manifestData)),
			},
		},
	}

	return writeJSON(filepath.Join(outputDir, "index.json"), index)
}

func copyFile(src, dst string, useSymlinks bool) error {
	if useSymlinks {
		absSrc, err := filepath.Abs(src)
		if err != nil {
			return err
		}
		return os.Symlink(absSrc, dst)
	}

	if err := os.Link(src, dst); err == nil {
		return nil
	}

	if err := tryReflink(src, dst); err == nil {
		return nil
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func hashBytes(data []byte) v1.Hash {
	h, _, _ := v1.SHA256(bytes.NewReader(data))
	return h
}

func assembleOCILayoutWithIndex(indexPath, outputDir string, manifestPaths, configPaths []string, layers layerMappingFlag, useSymlinks bool) error {
	if err := setupOCILayout(outputDir); err != nil {
		return err
	}

	// Build a map of available layers by their digest
	layerBlobsByDigest := make(map[string]string)
	for _, layer := range layers {
		metadataData, err := os.ReadFile(layer.metadata)
		if err != nil {
			return fmt.Errorf("reading layer metadata %s: %w", layer.metadata, err)
		}

		var metadata struct {
			Digest string `json:"digest"`
		}
		if err := json.Unmarshal(metadataData, &metadata); err != nil {
			return fmt.Errorf("unmarshaling layer metadata %s: %w", layer.metadata, err)
		}

		// Extract hex digest from sha256:xxxx format
		digest := strings.TrimPrefix(metadata.Digest, "sha256:")
		layerBlobsByDigest[digest] = layer.blob
	}

	blobs := make(blobMap)

	for i := range manifestPaths {
		manifestData, err := os.ReadFile(manifestPaths[i])
		if err != nil {
			return fmt.Errorf("reading manifest %d: %w", i, err)
		}

		var manifest v1.Manifest
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			return fmt.Errorf("unmarshaling manifest %d: %w", i, err)
		}

		// Add manifest to blobs
		manifestDigest := hashBytes(manifestData)
		blobs[manifestDigest.Hex] = manifestPaths[i]

		// Add config to blobs
		blobs[manifest.Config.Digest.Hex] = configPaths[i]

		// Only add layers that we have blobs for
		for _, layerDesc := range manifest.Layers {
			if blobPath, ok := layerBlobsByDigest[layerDesc.Digest.Hex]; ok {
				blobs[layerDesc.Digest.Hex] = blobPath
			}
		}
	}

	blobsDir := filepath.Join(outputDir, "blobs", "sha256")
	if err := copyBlobs(blobs, blobsDir, useSymlinks); err != nil {
		return err
	}

	// Copy the index file unmodified
	return copyFile(indexPath, filepath.Join(outputDir, "index.json"), false)
}

func setupOCILayout(outputDir string) error {
	blobsDir := filepath.Join(outputDir, "blobs", "sha256")
	if err := os.MkdirAll(blobsDir, 0755); err != nil {
		return fmt.Errorf("creating blobs directory: %w", err)
	}

	ociLayout := map[string]string{
		"imageLayoutVersion": OCILayoutVersion,
	}
	return writeJSON(filepath.Join(outputDir, "oci-layout"), ociLayout)
}

func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", path, err)
	}
	return os.WriteFile(path, data, 0644)
}

func copyBlobs(blobs blobMap, blobsDir string, useSymlinks bool) error {
	for digest, srcPath := range blobs {
		dstPath := filepath.Join(blobsDir, digest)
		if err := copyFile(srcPath, dstPath, useSymlinks); err != nil {
			return fmt.Errorf("copying blob %s: %w", digest, err)
		}
	}
	return nil
}
