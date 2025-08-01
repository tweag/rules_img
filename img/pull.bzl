"""Public API for pulling base container images."""

load("//img/private/repository_rules:pull.bzl", _pull = "pull")

pull = _pull
