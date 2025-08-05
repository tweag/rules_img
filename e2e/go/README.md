# Go Container Image Examples

This directory contains examples demonstrating different ways of building container images for Go applications using `rules_img`. The examples are ordered by increasing complexity, showcasing various features and use cases.

## Examples

### 1. [image/](./image/) - Basic Single-Platform Image

The simplest example showing how to build a container image for a Go application:
- Creates a single-platform container image with a Go binary
- Uses the native platform (whatever platform Bazel is running on)
- Demonstrates basic `image_layer` and `image_manifest` usage
- Shows how to push images to a registry

### 2. [cross/](./cross/) - Cross-Platform Building

Demonstrates cross-compilation capabilities:
- Uses `go_cross_binary` to build for a specific target platform (Linux ARM64)
- Shows how to build container images for different architectures than the host
- Explicitly sets platform metadata in the image manifest

### 3. [multiarch-transition/](./multiarch-transition/) - Automated Multi-Architecture Images

The recommended approach for multi-platform container images:
- Uses Bazel's platform transitions for automatic multi-platform builds
- Creates an `image_index` containing both ARM64 and AMD64 variants
- Minimal configuration - Bazel handles the platform-specific builds automatically
- Best choice when you want the same build configuration for all platforms

### 4. [multiarch-manual/](./multiarch-manual/) - Manual Multi-Architecture Images

For cases requiring full control over each platform variant:
- Explicitly defines separate binary targets for each architecture
- Manually creates platform-specific layers and manifests
- Assembles the multi-platform index from individual components
- Useful when different platforms need different configurations or build flags

## Running the Examples

Each example can be built and pushed using standard Bazel commands:

```bash
# Build an image
bazel build //image:go_image

# Push to a registry
bazel run //image:go_push

# Build all examples
bazel build //...
```
