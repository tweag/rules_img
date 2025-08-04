# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`rules_img` is a Bazel ruleset for building OCI-compliant container images. It provides a modern, Bazel-native alternative to Docker-based container building with advanced features like content addressable storage integration and multi-platform support.

## Build System & Commands

This project uses **Bazel with Bzlmod** (MODULE.bazel) as the primary build system.

### Core Commands
```bash
# Build all targets
bazel build //...

# Build specific components
bazel build //cmd/img //pkg/... //img/...

# Generate BUILD.bazel files
bazel run //util:gazelle

# Run benchmarks/examples
cd benchmark && bazel build //examples:my_image
cd benchmark && bazel run //examples:my_push

# Development environment (requires Nix)
nix develop
```

### Testing Strategy
This codebase uses integration testing through benchmark examples rather than traditional unit tests. Test by building and running example images in `benchmark/examples/`.

## Architecture Overview

### Core Bazel Rules (img/)
- `image_layer`: Creates container image layers from files
- `image_manifest`: Assembles single-platform container images
- `image_index`: Creates multi-platform container image indexes
- `image_push`: Pushes images to registries

### Provider System (img/private/providers/)
Bazel providers that pass metadata between rules:
- `LayerInfo`: Layer metadata (digest, size, media type)
- `ImageManifestInfo`: Single-platform image information
- `ImageIndexInfo`: Multi-platform image index information

### Go CLI Tools (cmd/)
Multiple specialized binaries orchestrated by the main `img` command:
- `layer`: Creates layers from file specifications
- `manifest`: Assembles image manifests and configs
- `push`: Registry pushing with multiple backends
- `compress`: Layer compression utilities

### Go Library Packages (pkg/)
- `push/`: Multiple push implementations (eager, lazy, CAS-based)
- `cas/`: Content Addressable Storage implementation with REAPI integration
- `serve/`: Registry serving (S3, upstream, REAPI backends)
- `compress/`: Compression utilities
- `auth/`: Authentication helpers

## Key Technical Features

### Content Addressable Storage (CAS)
The project integrates with Remote Execution API for efficient blob management. CAS-related code spans `pkg/cas/`, push implementations, and registry serving.

### Lazy Push Optimization
Avoids redundant uploads by checking registry state first. Implemented across push backends in `pkg/push/`.

### Multi-Platform Support
Uses Bazel platform transitions for cross-platform building. Platform-specific logic is handled in rule implementations.

### Registry Backend Flexibility
Supports multiple storage backends (S3, traditional registries, local serving) through pluggable architecture in `pkg/serve/`.

## Development Notes

### Dependencies
- Go 1.24.2 for tooling
- Custom fork of `go-containerregistry` with patches
- Bazel toolchains for cross-platform compatibility

### Architecture Patterns
- **Toolchain-based**: Uses Bazel toolchains for cross-platform builds
- **Provider-based information flow**: Clean separation between rules and data structures
- **Command composition**: Main `img` binary dispatches to specialized subcommands
- **Modular backends**: Pluggable registry and storage implementations

### Performance Focus
Multiple optimization strategies including lazy push, content addressability, and blob caching for large-scale container building scenarios.
