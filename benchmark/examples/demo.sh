#!/usr/bin/env bash

set -euo pipefail

repo_cache="$(bazel info repository_cache)"
if [[ "$repo_cache" != *"cache/repos"* ]]; then
    echo "Repository cache path is not set correctly."
    exit 1
fi

cleanup_cache() {
    bazel clean --expunge
    if [[ -d "$repo_cache" ]]; then
        echo "Cleaning up repository cache..."
        rm -rf "$repo_cache"
    fi
    bazel info > /dev/null
}

cleanup_cache
time bazel build @cuda --credential_helper=credential-helper

cleanup_cache
time bazel build @cuda_rules_oci
