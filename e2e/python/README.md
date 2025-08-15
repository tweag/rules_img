# Python e2e example

This directory contains an example of creating OCI container images for Python applications using `rules_img`.

## Building

```bash
bazel build //image:python_image
```

## Pushing to registry

```bash
bazel run //image:push
```

## Running locally

```bash
bazel run //image:push
docker run --rm ghcr.io/malt3/rules_img/python:native
```

## Multi-arch builds

```bash
bazel run //multiarch-transition:push
docker run --rm ghcr.io/malt3/rules_img/python:multiarch
```

See the BUILD files for more details.
