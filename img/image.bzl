"""Rules to build container images from layers.

Use `image_manifest` to create a single-platform container image,
and `image_index` to compose a multi-platform container image index.
"""

load("//img/private:index.bzl", _image_index = "image_index")
load("//img/private:manifest.bzl", _image_manifest = "image_manifest")

image_manifest = _image_manifest
image_index = _image_index
