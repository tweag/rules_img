package push

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	registryv1 "github.com/malt3/go-containerregistry/pkg/v1"
	"github.com/malt3/go-containerregistry/pkg/v1/types"

	"github.com/tweag/rules_img/pkg/api"
	registrytypes "github.com/tweag/rules_img/pkg/serve/registry/types"
)

var requestPath string

func PushMetadataProcess(ctx context.Context, args []string) {
	flagSet := flag.NewFlagSet("push-metadata", flag.ExitOnError)
	flagSet.Usage = func() {
		fmt.Fprintf(flagSet.Output(), "Writes metadata about a push operation.\n\n")
		fmt.Fprintf(flagSet.Output(), "Usage: img push-metadata [--from-file=request_file] [output]\n")
		flagSet.PrintDefaults()
		examples := []string{
			"img push-metadata --from-file push_request.json push_metadata.json",
		}
		fmt.Fprintf(flagSet.Output(), "\nExamples:\n")
		for _, example := range examples {
			fmt.Fprintf(flagSet.Output(), "  $ %s\n", example)
		}
		os.Exit(1)
	}
	flagSet.StringVar(&requestPath, "from-file", "", `Path to the request file.`)
	if err := flagSet.Parse(args); err != nil {
		flagSet.Usage()
		os.Exit(1)
	}
	if flagSet.NArg() != 1 {
		flagSet.Usage()
		os.Exit(1)
	}
	outputPath := flagSet.Arg(0)
	if requestPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --from-file is required")
		flagSet.Usage()
		os.Exit(1)
	}
	if err := WritePushMetadata(ctx, requestPath, outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing push metadata: %v\n", err)
		os.Exit(1)
	}
}

func WritePushMetadata(ctx context.Context, requestPath, outputPath string) error {
	rawRequest, err := os.ReadFile(requestPath)
	if err != nil {
		return fmt.Errorf("reading request file: %w", err)
	}
	var req request
	if err := json.Unmarshal(rawRequest, &req); err != nil {
		return fmt.Errorf("unmarshalling request file: %w", err)
	}

	descriptors, err := describeAll(req)
	if err != nil {
		return fmt.Errorf("calculating descriptors: %w", err)
	}

	metadata := pushMetadata{
		Command: req.Command,
		PushRequest: registrytypes.PushRequest{
			Strategy:   req.Strategy,
			Blobs:      descriptors,
			PushTarget: req.PushTarget,
			PullInfo:   req.PullInfo,
		},
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshalling metadata: %w", err)
	}
	if err := os.WriteFile(outputPath, metadataBytes, 0o644); err != nil {
		return fmt.Errorf("writing metadata file: %w", err)
	}
	return nil
}

func describeAll(req request) ([]api.Descriptor, error) {
	var descriptors []api.Descriptor
	if req.Manifest.ManifestPath != "" {
		var err error
		descriptors, err = appendManifestDescriptors(req.Manifest.ManifestPath, descriptors)
		if err != nil {
			return nil, fmt.Errorf("getting manifest descriptors: %w", err)
		}
	} else if req.Index.IndexPath != "" {
		var err error
		descriptors, err = appendIndexDescriptors(req.Index, descriptors)
		if err != nil {
			return nil, fmt.Errorf("getting index descriptors: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no manifest or index path provided")
	}
	return descriptors, nil
}

func appendIndexDescriptors(req indexRequest, descriptors []api.Descriptor) ([]api.Descriptor, error) {
	indexDescriptor, err := describeJSONFile(req.IndexPath)
	if err != nil {
		return nil, fmt.Errorf("getting index descriptor: %w", err)
	}
	var index registryv1.IndexManifest
	rawIndex, err := os.ReadFile(req.IndexPath)
	if err != nil {
		return nil, fmt.Errorf("reading index file: %w", err)
	}
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		return nil, fmt.Errorf("unmarshalling index file: %w", err)
	}

	descriptors = append(descriptors, toAPIDescriptor(indexDescriptor))
	for _, desc := range index.Manifests {
		descriptors = append(descriptors, toAPIDescriptor(desc))
	}
	for _, manifest := range req.Manifests {
		var err error
		descriptors, err = appendManifestChildren(manifest.ManifestPath, descriptors)
		if err != nil {
			return nil, fmt.Errorf("getting manifest children: %w", err)
		}
	}
	return descriptors, nil
}

func appendManifestDescriptors(filePath string, descriptors []api.Descriptor) ([]api.Descriptor, error) {
	manifestDescriptor, err := describeJSONFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("getting manifest descriptor: %w", err)
	}

	descriptors = append(descriptors, toAPIDescriptor(manifestDescriptor))
	return appendManifestChildren(filePath, descriptors)
}

func appendManifestChildren(filePath string, descriptors []api.Descriptor) ([]api.Descriptor, error) {
	var manifest registryv1.Manifest
	rawManifest, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading manifest file: %w", err)
	}
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshalling manifest file: %w", err)
	}

	descriptors = append(descriptors, toAPIDescriptor(manifest.Config))
	for _, layer := range manifest.Layers {
		descriptors = append(descriptors, toAPIDescriptor(layer))
	}
	return descriptors, nil
}

func describeJSONFile(filePath string) (registryv1.Descriptor, error) {
	rawFile, err := os.ReadFile(filePath)
	if err != nil {
		return registryv1.Descriptor{}, fmt.Errorf("reading file: %w", err)
	}
	var file mediaTyped
	if err := json.Unmarshal(rawFile, &file); err != nil {
		return registryv1.Descriptor{}, fmt.Errorf("unmarshalling file: %w", err)
	}
	descriptor, err := descriptorForFile(filePath, file.MediaType)
	if err != nil {
		return registryv1.Descriptor{}, fmt.Errorf("getting descriptor: %w", err)
	}
	return descriptor, nil
}

func descriptorForFile(filePath string, mediaType string) (registryv1.Descriptor, error) {
	fileHandle, err := os.Open(filePath)
	if err != nil {
		return registryv1.Descriptor{}, fmt.Errorf("opening file: %w", err)
	}
	defer fileHandle.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, fileHandle)
	if err != nil {
		return registryv1.Descriptor{}, fmt.Errorf("reading file: %w", err)
	}
	digest := hasher.Sum(nil)
	return registryv1.Descriptor{
		MediaType: types.MediaType(mediaType),
		Digest:    registryv1.Hash{Algorithm: "sha256", Hex: fmt.Sprintf("%x", digest)},
		Size:      size,
	}, nil
}

func toAPIDescriptor(d registryv1.Descriptor) api.Descriptor {
	return api.Descriptor{
		MediaType: string(d.MediaType),
		Digest:    d.Digest.String(),
		Size:      d.Size,
	}
}

type pushMetadata struct {
	Command string `json:"command"`
	registrytypes.PushRequest
}

type mediaTyped struct {
	MediaType string `json:"mediaType"`
}
