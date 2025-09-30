#!/usr/bin/env bash

# This script is invoked by release.yaml

set -euo pipefail

rm -rf dist

# Build the distribution tarball
echo "Building distribution tarball..." 1>&2
bazel build //img/private/release:dist_tar 1>&2

# Get the output file location using bazel cquery
TARBALL=$(bazel cquery --output=files //img/private/release:dist_tar)

# Create dist directory if it doesn't exist
mkdir -p dist

# Extract the tarball to the dist directory
echo "Extracting tarball to dist directory..." 1>&2
tar -xvf "$TARBALL" -C dist 1>&2

echo "Release preparation completed. Distribution files are in the 'dist' directory." 1>&2

# Generate release notes using Bazel
echo "Generating release notes using Bazel..." 1>&2

# Build release notes using Bazel
bazel build --output_groups=release_notes //img/private/release:versioned_src_tar 1>&2

# Get the release notes file location using bazel cquery
RELEASE_NOTES_FILE=$(bazel cquery --output=files //img/private/release:versioned_src_tar --output_groups=release_notes)

# Output release notes to stdout
cat "$RELEASE_NOTES_FILE"

# Add generated API docs to the release, see https://github.com/bazelbuild/bazel-central-registry/issues/5593
docs="$(mktemp -d)"; targets="$(mktemp)"
bazel --output_base="$docs" query --output=label --output_file="$targets" 'kind("starlark_doc_extract rule", //...)'
bazel --output_base="$docs" build --target_pattern_file="$targets"
tar --create --auto-compress \
    --directory "$(bazel --output_base="$docs" info bazel-bin)" \
    --file "$GITHUB_WORKSPACE/${ARCHIVE%.tar.gz}.docs.tar.gz" .
