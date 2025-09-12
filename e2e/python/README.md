# Python e2e example

This directory contains an example of creating OCI container images for Python applications using `rules_img`.

## Building

```bash
bazel build //:image
```

## Pushing to registry

```bash
bazel run //:push
```

## Running locally

```bash
bazel run //:load
docker run --rm ghcr.io/malt3/rules_img/python:sideloaded
```

See the BUILD files for more details.
