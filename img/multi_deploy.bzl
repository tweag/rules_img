"""Public API for container image multi deploy rule."""

load("//img/private:multi_deploy.bzl", _multi_deploy = "multi_deploy")

multi_deploy = _multi_deploy
