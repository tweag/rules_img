package manifest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/tweag/rules_img/pkg/api"
)

var (
	operatingSystem       string
	architecture          string
	layerFromMetadataArgs fileList
	configFragment        string
	baseManifest          string
	baseConfig            string
	manifestOutput        string
	configOutput          string
	descriptorOutput      string
)

func ManifestProcess(_ context.Context, args []string) {
	flagSet := flag.NewFlagSet("manifest", flag.ExitOnError)
	flagSet.Usage = func() {
		fmt.Fprintf(flagSet.Output(), "Creates an OCI image config and manifest based on layers and other metadata.\n\n")
		fmt.Fprintf(flagSet.Output(), "Usage: img manifest [--os os] [--architecture arch] [--layer-from-metadata param_file] [--config-fragment config_file] [--base-manifest manifest_file] [--base-config config_file] [--manifest manifest_file] [--config config_file]\n")
		flagSet.PrintDefaults()
		examples := []string{
			"img manifest --os linux --architecture amd64 --layer-from-metadata layer-metadata.json --config-fragment extra-config.json --base-manifest base-manifest.json --base-config base-config.json --manifest manifest.json --config config.json",
		}
		fmt.Fprintf(flagSet.Output(), "\nExamples:\n")
		for _, example := range examples {
			fmt.Fprintf(flagSet.Output(), "  $ %s\n", example)
		}
		os.Exit(1)
	}
	flagSet.StringVar(&operatingSystem, "os", "linux", `The operating system of the image. Defaults to linux.`)
	flagSet.StringVar(&architecture, "architecture", "amd64", `The architecture of the image. Defaults to amd64.`)
	flagSet.Var(&layerFromMetadataArgs, "layer-from-metadata", `Ordered list of layer metadata files that will make up the image, as produced by "img layer --metadata".`)
	flagSet.StringVar(&configFragment, "config-fragment", "", `A JSON file containing a config fragment to be merged into the final config. This is useful for adding custom labels or other metadata to the image.`)
	flagSet.StringVar(&baseManifest, "base-manifest", "", `A JSON file containing a base manifest to be merged into the final manifest. This is useful for adding custom layers or other metadata to the image.`)
	flagSet.StringVar(&baseConfig, "base-config", "", `A JSON file containing a base config to be merged into the final config. This is useful for adding custom labels or other metadata to the image.`)
	flagSet.StringVar(&manifestOutput, "manifest", "", `The output file for the final manifest.`)
	flagSet.StringVar(&configOutput, "config", "", `The output file for the final config.`)
	flagSet.StringVar(&descriptorOutput, "descriptor", "", `The output file for the descriptor of the manifest.`)

	if err := flagSet.Parse(args); err != nil {
		flagSet.Usage()
		os.Exit(1)
	}
	if flagSet.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "Unexpected positional arguments: %s\n", strings.Join(flagSet.Args(), " "))
		flagSet.Usage()
		os.Exit(1)
	}

	layers := make([]api.LayerMetadata, len(layerFromMetadataArgs))
	for i, layerFile := range layerFromMetadataArgs {
		layer, err := readLayerMetadata(layerFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read layer metadata file %s: %v\n", layerFile, err)
			os.Exit(1)
		}
		layers[i] = layer
	}

	config, err := prepareConfig(layers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to prepare config: %v\n", err)
		os.Exit(1)
	}

	configRaw, err := json.Marshal(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal config: %v\n", err)
		os.Exit(1)
	}
	sha256Hash := sha256.Sum256(configRaw)

	layerDescriptors := make([]specv1.Descriptor, len(layers))
	for i, layer := range layers {
		layerDescriptors[i] = specv1.Descriptor{
			MediaType: layer.MediaType,
			Digest:    digest.Digest(layer.Digest),
			Size:      layer.Size,
		}
	}

	manifest := specv1.Manifest{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType: specv1.MediaTypeImageManifest,
		Config: specv1.Descriptor{
			MediaType: specv1.MediaTypeImageConfig,
			Digest:    digest.NewDigestFromBytes(digest.SHA256, sha256Hash[:]),
			Size:      int64(len(configRaw)),
		},
		Layers: layerDescriptors,
	}

	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal manifest: %v\n", err)
		os.Exit(1)
	}

	manifestSHA256 := sha256.Sum256(manifestRaw)
	descriptor := specv1.Descriptor{
		MediaType: specv1.MediaTypeImageManifest,
		Digest:    digest.NewDigestFromBytes(digest.SHA256, manifestSHA256[:]),
		Size:      int64(len(manifestRaw)),
		Platform: &specv1.Platform{
			Architecture: architecture,
			OS:           operatingSystem,
		},
	}
	descriptorRaw, err := json.Marshal(descriptor)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal manifest descriptor: %v\n", err)
		os.Exit(1)
	}

	if manifestOutput != "" {
		if err := os.WriteFile(manifestOutput, manifestRaw, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write manifest to %s: %v\n", manifestOutput, err)
			os.Exit(1)
		}
	}
	if configOutput != "" {
		if err := os.WriteFile(configOutput, configRaw, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write config to %s: %v\n", configOutput, err)
			os.Exit(1)
		}
	}
	if descriptorOutput != "" {
		if err := os.WriteFile(descriptorOutput, descriptorRaw, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write manifest descriptor to %s: %v\n", descriptorOutput, err)
			os.Exit(1)
		}
	}
}

func prepareConfig(layers []api.LayerMetadata) (specv1.Image, error) {
	// first, read the base config
	// then, layer the config fragment on top of it
	// finally, add our own stuff

	var config specv1.Image
	if baseConfig != "" {
		if err := overlayConfigFromFile(&config, baseConfig, true); err != nil {
			return config, fmt.Errorf("reading base config: %w", err)
		}
	}
	if configFragment != "" {
		if err := overlayConfigFromFile(&config, configFragment, false); err != nil {
			return config, fmt.Errorf("reading config fragment: %w", err)
		}
	}

	if err := overlayNewConfigValues(&config, layers); err != nil {
		return config, fmt.Errorf("overlaying new config values: %w", err)
	}
	return config, nil
}

func readLayerMetadata(filePath string) (api.LayerMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return api.LayerMetadata{}, fmt.Errorf("opening layer metadata file: %w", err)
	}
	defer file.Close()

	var layer api.LayerMetadata
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&layer); err != nil {
		return api.LayerMetadata{}, fmt.Errorf("decoding layer metadata file: %w", err)
	}

	return layer, nil
}

func overlayConfigFromFile(config *specv1.Image, filePath string, isBase bool) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening config file: %w", err)
	}
	defer file.Close()

	var configFragment specv1.Image
	if err := json.NewDecoder(file).Decode(&configFragment); err != nil {
		return fmt.Errorf("decoding config file: %w", err)
	}

	// when merging, we need to perform some checks first
	if configFragment.OS != "" && config.OS != "" && configFragment.OS != config.OS {
		return fmt.Errorf("OS mismatch: %s != %s", configFragment.OS, config.OS)
	}
	if configFragment.Architecture != "" && config.Architecture != "" && configFragment.Architecture != config.Architecture {
		return fmt.Errorf("architecture mismatch: %s != %s", configFragment.Architecture, config.Architecture)
	}

	// merge the config fragment into the base config
	if configFragment.OS != "" {
		config.OS = configFragment.OS
	}
	if configFragment.Architecture != "" {
		config.Architecture = configFragment.Architecture
	}
	if len(configFragment.History) > 0 {
		config.History = append(config.History, configFragment.History...)
	}

	// merge config.Config
	if configFragment.Config.User != "" {
		config.Config.User = configFragment.Config.User
	}
	if configFragment.Config.ExposedPorts != nil {
		// replace the ExposedPorts map
		// so that we can unexpose ports
		// that were exposed in the underlying config
		config.Config.ExposedPorts = maps.Clone(configFragment.Config.ExposedPorts)
	}
	if configFragment.Config.Env != nil {
		// for environment variables, we need to replace items thar are in both
		// configs, but append new ones
		keysUnderlying := make(map[string]string, len(config.Config.Env))
		keysOverlay := make(map[string]string, len(configFragment.Config.Env))
		for _, env := range config.Config.Env {
			kv := strings.SplitN(env, "=", 2)
			if len(kv) != 2 {
				return fmt.Errorf("invalid environment variable format: %s (should be key=value)", env)
			}
			keysUnderlying[kv[0]] = kv[1]
		}
		for _, env := range configFragment.Config.Env {
			kv := strings.SplitN(env, "=", 2)
			if len(kv) != 2 {
				return fmt.Errorf("invalid environment variable format: %s (should be key=value)", env)
			}
			keysOverlay[kv[0]] = kv[1]
		}
		// replace the keys in the underlying config
		for i, env := range config.Config.Env {
			kv := strings.SplitN(env, "=", 2)
			if _, ok := keysOverlay[kv[0]]; ok {
				config.Config.Env[i] = fmt.Sprintf("%s=%s", kv[0], keysOverlay[kv[0]])
				delete(keysOverlay, kv[0])
			}
		}

		// append the new keys in the original order
		for _, env := range configFragment.Config.Env {
			kv := strings.SplitN(env, "=", 2)
			if _, ok := keysUnderlying[kv[0]]; !ok {
				config.Config.Env = append(config.Config.Env, env)
			}
		}
	}
	if configFragment.Config.Entrypoint != nil {
		config.Config.Entrypoint = slices.Clone(configFragment.Config.Entrypoint)
	}
	if configFragment.Config.Cmd != nil {
		config.Config.Cmd = slices.Clone(configFragment.Config.Cmd)
	}
	if configFragment.Config.Volumes != nil {
		config.Config.Volumes = maps.Clone(configFragment.Config.Volumes)
	}
	if configFragment.Config.WorkingDir != "" {
		config.Config.WorkingDir = configFragment.Config.WorkingDir
	}
	if configFragment.Config.Labels != nil {
		// merge labels
		if config.Config.Labels == nil {
			config.Config.Labels = maps.Clone(configFragment.Config.Labels)
		} else {
			for k, v := range configFragment.Config.Labels {
				config.Config.Labels[k] = v
			}
		}
	}
	if configFragment.Config.StopSignal != "" {
		config.Config.StopSignal = configFragment.Config.StopSignal
	}

	// inherit some fields if this is not a base config
	if !isBase {
		if !configFragment.Created.IsZero() {
			config.Created = configFragment.Created
		}
		if configFragment.Author != "" {
			config.Author = configFragment.Author
		}
	}

	return nil
}

func overlayNewConfigValues(config *specv1.Image, layers []api.LayerMetadata) error {
	if config.OS == "" && operatingSystem != "" && config.OS != operatingSystem {
		return fmt.Errorf("OS mismatch: %s != %s", config.OS, operatingSystem)
	}
	if config.OS == "" {
		config.OS = operatingSystem
	}
	if config.Architecture == "" && architecture != "" && config.Architecture != architecture {
		return fmt.Errorf("architecture mismatch: %s != %s", config.Architecture, architecture)
	}
	if config.Architecture == "" {
		config.Architecture = architecture
	}

	// Set the rootfs struct
	config.RootFS.Type = "layers"
	config.RootFS.DiffIDs = make([]digest.Digest, len(layers))
	for i, layer := range layers {
		config.RootFS.DiffIDs[i] = digest.Digest(layer.DiffID)
	}
	return nil
}
