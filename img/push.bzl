"""Public API for container image push rules."""

load("//img/private:push.bzl", _image_push = "image_push")

image_push = _image_push
