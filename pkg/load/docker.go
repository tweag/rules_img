package load

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/bazelbuild/rules_go/go/runfiles"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/tweag/rules_img/pkg/api"
	"github.com/tweag/rules_img/pkg/docker"
)

// LoadViaDocker loads images using docker load with streaming
func LoadViaDocker(ctx context.Context, rf *runfiles.Runfiles, req *Request) error {
	// Create a pipe to stream the tar to docker load
	pr, pw := io.Pipe()

	// Start docker load in the background
	errCh := make(chan error, 1)
	go func() {
		err := docker.Load(pr)
		pr.Close()
		errCh <- err
	}()

	// Stream the tar to the pipe writer
	err := streamDockerTar(ctx, rf, req, pw)
	pw.Close() // Always close, even on error

	// Wait for docker load to complete
	loadErr := <-errCh

	// Return the first error
	if err != nil {
		return err
	}
	return loadErr
}

func streamDockerTar(ctx context.Context, rf *runfiles.Runfiles, req *Request, w io.Writer) error {
	tw := docker.NewTarWriter(w)

	if req.Index != nil {
		// For multi-platform images, we need to select a manifest
		manifestIndex, err := selectManifestForPlatform(rf, req.Index, req.Platforms)
		if err != nil {
			return err
		}
		return streamManifestToTar(ctx, rf, &req.Index.Manifests[manifestIndex], req.Strategy, req.Tag, tw)
	} else if req.Manifest != nil {
		return streamManifestToTar(ctx, rf, req.Manifest, req.Strategy, req.Tag, tw)
	}

	return fmt.Errorf("no manifest or index provided")
}

func streamManifestToTar(ctx context.Context, rf *runfiles.Runfiles, manifest *ManifestRequest, strategy string, tag string, tw *docker.TarWriter) error {
	// Load config
	configPath, err := rf.Rlocation(manifest.ConfigPath)
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	// Write config
	if err := tw.WriteConfig(configData); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Set tags
	if tag != "" {
		tw.SetTags([]string{NormalizeDockerReference(tag)})
	}

	// Load manifest to get layer descriptors
	manifestPath, err := rf.Rlocation(manifest.ManifestPath)
	if err != nil {
		return fmt.Errorf("resolving manifest path: %w", err)
	}
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	var ociManifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &ociManifest); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	// Stream layers
	if err := streamLayers(ctx, rf, manifest, &ociManifest, strategy, tw); err != nil {
		return fmt.Errorf("streaming layers: %w", err)
	}

	// Finalize the tar
	if err := tw.Finalize(); err != nil {
		return fmt.Errorf("finalizing tar: %w", err)
	}

	// Print the tag
	if tag != "" {
		fmt.Println(NormalizeDockerReference(tag))
	}

	return nil
}

func streamLayers(ctx context.Context, rf *runfiles.Runfiles, manifest *ManifestRequest, ociManifest *ocispec.Manifest, strategy string, tw *docker.TarWriter) error {
	var casReader BlobReader
	if strategy == "lazy" {
		var err error
		casReader, err = setupCASReader(strategy)
		if err != nil {
			return err
		}
	}

	layerPathIndex := 0
	for i, layerDesc := range ociManifest.Layers {
		// Get reader for the layer
		reader, err := getLayerReader(rf, manifest, &layerDesc, &layerPathIndex, strategy, casReader)
		if err != nil {
			return fmt.Errorf("getting reader for layer %d: %w", i, err)
		}
		defer reader.Close()

		// Stream the layer to the tar
		if err := tw.WriteLayer(layerDesc.Digest, layerDesc.Size, reader); err != nil {
			return fmt.Errorf("writing layer %d: %w", i, err)
		}
	}

	return nil
}

func getLayerReader(rf *runfiles.Runfiles, manifest *ManifestRequest, layerDesc *ocispec.Descriptor, layerPathIndex *int, strategy string, casReader BlobReader) (io.ReadCloser, error) {
	isMissing := manifest.MissingBlobs != nil && contains(manifest.MissingBlobs, layerDesc.Digest.Hex())
	apiDesc := api.Descriptor{
		Digest: layerDesc.Digest.String(),
		Size:   layerDesc.Size,
	}

	if strategy == "lazy" && casReader != nil {
		return GetCASLayerReader(apiDesc, casReader)
	} else if !isMissing && *layerPathIndex < len(manifest.Layers) {
		path := manifest.Layers[*layerPathIndex]
		*layerPathIndex++
		resolvedPath, err := rf.Rlocation(path)
		if err != nil {
			return nil, fmt.Errorf("resolving layer path %s: %w", path, err)
		}
		return os.Open(resolvedPath)
	} else if isMissing {
		return GetRemoteLayerReader(apiDesc, manifest.PullInfo)
	}

	return nil, fmt.Errorf("no source for layer %s", layerDesc.Digest)
}

// selectManifestForPlatform selects the appropriate manifest from an index based on platform criteria
func selectManifestForPlatform(rf *runfiles.Runfiles, index *IndexRequest, platforms []string) (int, error) {
	// Load and parse the index
	indexPath, err := rf.Rlocation(index.IndexPath)
	if err != nil {
		return 0, fmt.Errorf("resolving index path: %w", err)
	}
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return 0, fmt.Errorf("reading index: %w", err)
	}

	var ociIndex ocispec.Index
	if err := json.Unmarshal(indexData, &ociIndex); err != nil {
		return 0, fmt.Errorf("parsing index: %w", err)
	}

	// If no platforms specified and only one manifest, use that
	if len(platforms) == 0 && len(ociIndex.Manifests) == 1 {
		return 0, nil
	}

	// If no platform specified, use current platform
	if len(platforms) == 0 {
		platforms = []string{runtime.GOOS + "/" + runtime.GOARCH}
	}

	// Find matching manifest
	for i, manifestDesc := range ociIndex.Manifests {
		if manifestDesc.Platform != nil && platformMatches(manifestDesc.Platform, platforms) {
			return i, nil
		}
	}

	return 0, fmt.Errorf("no manifest found for platform(s): %v", platforms)
}
