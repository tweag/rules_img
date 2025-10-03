#!/usr/bin/env bash
exec bazel run -- @rules_go//go/tools/gopackagesdriver "${@}"
