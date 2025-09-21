#!/usr/bin/env bash
exec nix run .#bazel-fhs -- run -- @rules_go//go/tools/gopackagesdriver "${@}"
