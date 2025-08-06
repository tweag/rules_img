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
