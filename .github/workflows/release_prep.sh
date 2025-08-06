#!/usr/bin/env bash

# This script is invoked by release.yaml

set -euo pipefail

rm -rf dist

# Build the distribution tarball
echo "Building distribution tarball..."
bazel build //img/private/release:dist_tar

# Get the output file location using bazel cquery
TARBALL=$(bazel cquery --output=files //img/private/release:dist_tar)

# Create dist directory if it doesn't exist
mkdir -p dist

# Extract the tarball to the dist directory
echo "Extracting tarball to dist directory..."
tar -xvf "$TARBALL" -C dist

echo "Release preparation completed. Distribution files are in the 'dist' directory."
