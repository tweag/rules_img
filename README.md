# rules_img

Modern Bazel rules for building OCI container images with advanced performance optimizations.

## Features

- 🚀 **High Performance** - Multiple push strategies including lazy push and CAS integration
- 📦 **OCI Compliant** - Builds standard OCI images compatible with any container runtime
- 🔧 **Bazel Native** - No Docker daemon required, fully hermetic builds
- 🌍 **Multi-Platform** - Native cross-platform support through Bazel transitions
- ⚡ **eStargz Support** - Lazy pulling optimization for faster container starts
- 🪶 **Smaller layers** - Deduplicates files using hardlinks
- 🎯 **Shallow Base Images** - Avoid downloading layers from huge base images like CUDA
- 🏢 **Enterprise Ready** - Remote Build Execution and Content Addressable Storage integration

## Installation

Add to your `MODULE.bazel`:

```starlark
bazel_dep(name = "rules_img", version = "0.0.0")
```

Configure default settings (optional) in `.bazelrc`:

```
# The compressions algorithm to use ("gzip" or "zstd")
common --@rules_img//img/settings:compress=zstd

# Support for seekable eStargz layers
# with the containerd stargz-snapshotter
common --@rules_img//img/settings:estargz=enabled

# The push strategy to use (see below for more info).
# "eager", "lazy", "cas_registry", or "bes"
common --@rules_img//img/settings:push_strategy=eager
```

You also need a credential helper to pull base image from container registries.
We recommend [tweag-credential-helper][tweag-credential-helper].

## Quick Start

### Building a Simple Image

Add base image to `MODULE.bazel`:

```starlark
pull = use_repo_rule("@rules_img//img/private/repository_rules:pull.bzl", "pull")

pull(
    name = "ubuntu",
    digest = "sha256:1e622c5f073b4f6bfad6632f2616c7f59ef256e96fe78bf6a595d1dc4376ac02",
    registry = "index.docker.io",
    repository = "library/ubuntu",
    tag = "24.04",
)

pull(
    name = "cuda",
    digest = "sha256:f353ffca86e0cd93ab2470fe274ecf766519c24c37ed58cc2f91d915f7ebe53c",
    registry = "index.docker.io",
    repository = "nvidia/cuda",
    tag = "12.8.1-cudnn-devel-ubuntu20.04",
)
```

```starlark
load("@rules_img//img:layer.bzl", "image_layer")
load("@rules_img//img:image.bzl", "image_manifest")

# Create a layer from files
image_layer(
    name = "app_layer",
    srcs = {
        "/app/bin/server": "//cmd/server",
        "/app/config": "//configs:prod",
    },
    compress = "zstd",  # Use zstd compression (optional, uses global default otherwise)
)

# Build a container image
image_manifest(
    name = "app_image",
    base = "@ubuntu", # Optional: build "from scratch" without base.
    layers = [
        ":app_layer",
    ],
    config_fragment = "config.json",  # Optional image configuration, uses sane defaults.
)
```

### Multi-Platform Images

In most cases, you can just use the builtin transitions feature:

```starlark
load("@rules_img//img:image.bzl", "image_manifest", "image_index")

# Create platform-specific images
image_manifest(
    name = "app",
    layers = [":app_layer"],
)

# Combine into multi-platform index
image_index(
    name = "app",
    manifests = [":app_amd"],
    platforms = [
        "//:linux_amd64",
        "//:linux_arm64",
    ],
)
```

<details>
<summary>You are in full control over the images you put into an image index</summary>

```starlark
load("@rules_img//img:image.bzl", "image_manifest", "image_index")

# Create platform-specific images
image_manifest(
    name = "app_amd64",
    layers = [":app_layer"],
    architecture = "amd64",
    os = "linux",
)

image_manifest(
    name = "app_arm64",
    layers = [":app_layer"],
    architecture = "arm64",
    os = "linux",
)

# Combine into multi-platform index
image_index(
    name = "app",
    manifests = [
        ":app_amd64",
        ":app_arm64",
    ],
)
```

</details>

### Pushing to Registry

```starlark
load("@rules_img//img:push.bzl", "image_push")

image_push(
    name = "push",
    image = ":app",
    registry = "ghcr.io",
    repository = "my-project/app",
    tag = "latest",
)
```

Run with:
```bash
bazel run //:push
```

## Advanced Features

### eStargz Optimization

Enable lazy pulling for faster container startup:

```starlark
image_layer(
    name = "optimized_layer",
    srcs = {...},
    estargz = "enabled",  # Creates seekable compressed layers
)
```

The same setting can be globally enabled using `--@rules_img//img/settings:estargz=enabled`.
Read the [stargz-snapshotter documentation][stargz-snapshotter] for more information.

### Layer Optimization

Create layers from existing tarballs with optimization:

```starlark
load("@rules_img//img:layer.bzl", "layer_from_tar")

layer_from_tar(
    name = "base_layer",
    src = "@debian_base//file",
    compress = "zstd",
    optimize = True,  # Deduplicate tar contents
)
```

### Global Configuration

Configure defaults via command-line flags:

```bash
# Set default compression
bazel build //... --//img/settings:compress=zstd

# Enable eStargz by default
bazel build //... --//img/settings:estargz=enabled

# Use CAS-based registry push
bazel build //... --//img/settings:push_strategy=cas_registry
```

## Push Strategies

rules_img supports multiple push strategies optimized for different scenarios:

| Strategy | Description | Use Case | Requirements |
|----------|-------------|----------|--------------|
| `eager` | Traditional push, download all blobs to the machine running Bazel, then uploads all blobs. | Simple deployments | Normal container registry |
| `lazy` | Checks registry first, skips existing blobs and streams missing blobs from Bazel's remote cache | Faster CI/CD and Build without the Bytes | Bazel remote cache |
| `cas_registry` | Uses special container registry that is directly connected to Bazel's remote cache | Fast development cycles. | Special container registry (`cmd/registry`), Bazel remote cache |
| `bes` | Image push happens as side-effect of BES upload. Requires self-hosted BES server. | Extremely fast and efficient for large organizations. | Special BES backend (`cmd/bes`), Bazel remote cache |

## Comparison

### vs rules_oci

- ✅ [Shallow base image pulling](#shallow-base-image-pulling)
- ✅ [Layers are produced in a single action](#single-action-layers)
- ✅ [Deduplication of layer contents](#layer-optimization)
- ✅ [Advanced push strategies](#advanced-push-strategies)
- ✅ [eStargz support for lazy pulling](#estargz-lazy-pulling)

## Documentation

- [API Reference](docs/)
  - **Layer Rules**
    - [`image_layer`](docs/layer.md#image_layer) - Create layers from files
    - [`layer_from_tar`](docs/layer.md#layer_from_tar) - Create layers from tar archives
  - **Image Rules**
    - [`image_manifest`](docs/image.md#image_manifest) - Build single-platform images
    - [`image_index`](docs/image.md#image_index) - Build multi-platform image indexes
  - **Push Rules**
    - [`image_push`](docs/push.md#image_push) - Push images to registries

## Examples

See the [benchmark/examples/](benchmark/examples/) directory for complete examples.

## Key Differences Explained

### Shallow Base Image Pulling

Unlike rules_oci which downloads all layers of a base image, rules_img uses a "shallow pull" approach. When you reference a base image like CUDA (which can be 10+ GB), rules_img only downloads the manifest and config - not the actual layer blobs. The layers are only downloaded when and if they're needed during push operations.

This results in:
- **Faster builds** - No waiting for large base image downloads
- **Reduced bandwidth** - Only download what you actually use
- **True Build-without-the-bytes** - Other rulesets download base layers to your local machine in a repository rule. This step cannot be remotely executed and is repeated on every machine running Bazel.

Example with a large CUDA base image:
```starlark
# This won't download the 10GB of CUDA layers!
pull(
    name = "cuda",
    digest = "sha256:...",
    registry = "index.docker.io",
    repository = "nvidia/cuda",
)
```

### Single Action Layers

rules_img produces both the layer blob and its metadata in a single Bazel action. This design has several advantages:

- **Remote execution friendly** - Single action works better with RBE
- **Image Manifest only depends on metadata** - In rules_oci, image actions depend on the actual blobs of their base image and layers, which must be available during the manifest writing action.

The metadata includes the layer's digest, size, and diff ID, all computed during layer creation.

### Layer Optimization

When writing a tar layer, rules_img uses hardlinks to deduplicate identical files.
This allows for smaller container images.

### Advanced Push Strategies

While rules_oci uses a traditional push approach, rules_img offers four sophisticated strategies:

1. **Eager** - Traditional approach, similar to rules_oci
2. **Lazy** - Checks registry first, only uploads missing blobs
3. **CAS Registry** - Direct integration with Bazel's content-addressable storage
4. **BES** - Pushes as a side-effect of build event uploads

These strategies enable:
- **Faster CI/CD** - Lazy push can skip 90%+ of uploads in typical workflows
- **Build without the bytes** - CAS/BES strategies work with remote execution
- **Scalability** - Designed for organizations with thousands of builds per day

### eStargz Lazy Pulling

rules_img has first-class support for eStargz (enhanced stargz), enabling "lazy pulling" at container runtime. This means:

- **Instant container starts** - Containers can start before all layers download
- **Bandwidth savings** - Only accessed files are downloaded
- **Seekable layers** - Random access to files within compressed layers

Combined with containerd's stargz-snapshotter, this can reduce container startup time from minutes to seconds for large images.

```starlark
image_layer(
    name = "optimized_layer",
    srcs = {...},
    estargz = "enabled",  # Enable seekable compression
)
```

## Contributing

Contributions are welcome! Please read our contributing guidelines and submit pull requests to our repository.

## License

This project is licensed under the Apache License 2.0 - see the LICENSE file for details.

[tweag-credential-helper]: https://github.com/tweag/credential-helper
[stargz-snapshotter]: https://github.com/containerd/stargz-snapshotter
