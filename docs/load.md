<!-- Generated with Stardoc: http://skydoc.bazel.build -->

Public API for loading container images into a daemon.

The `image_load` rule creates an executable target that loads container images into a local daemon (containerd or Docker).

## Example

```python
load("@rules_img//img:image.bzl", "image_manifest")
load("@rules_img//img:load.bzl", "image_load")
load("@rules_img//img:layer.bzl", "image_layer")

# Create a simple layer
image_layer(
    name = "app_layer",
    srcs = {
        "/app/hello.txt": "hello.txt",
    },
)

# Build an image
image_manifest(
    name = "my_image",
    base = "@alpine",
    layers = [":app_layer"],
)

# Create a load target
image_load(
    name = "load",
    image = ":my_image",
    tag = "my-app:latest",
)
```

Then run:
```bash
# Load the image into your local daemon
bazel run //:load
```

## Platform Selection

When running the load target, you can use the `--platform` flag to filter which platforms to load from multi-platform images:

```bash
# Load all platforms (default)
bazel run //path/to:load_target

# Load only linux/amd64
bazel run //path/to:load_target -- --platform linux/amd64
```

**Note**: Docker daemon only supports loading a single platform at a time. If multiple platforms are specified with Docker, an error will be returned.

<a id="image_load"></a>

## image_load

<pre>
load("@rules_img//img:load.bzl", "image_load")

image_load(<a href="#image_load-name">name</a>, <a href="#image_load-build_settings">build_settings</a>, <a href="#image_load-daemon">daemon</a>, <a href="#image_load-image">image</a>, <a href="#image_load-stamp">stamp</a>, <a href="#image_load-strategy">strategy</a>, <a href="#image_load-tag">tag</a>)
</pre>



**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="image_load-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="image_load-build_settings"></a>build_settings |  Build settings to use for [template expansion](/docs/templating.md). Keys are setting names, values are labels to string_flag targets.   | Dictionary: String -> Label | optional |  `{}`  |
| <a id="image_load-daemon"></a>daemon |  Container daemon to use for loading the image.<br><br>- `auto`: Uses the global default setting (usually `docker`) - `containerd`: Loads directly into containerd. Supports multi-platform images. - `docker`: Loads via Docker daemon (but uses containerd backend if available). Falls back to slower `docker load` if Docker doesn't use containerd storage. `docker load` is limited to single-platform images.<br><br>**Platform Selection**: The load binary supports a `--platform` flag to filter which platforms to load: - No flag: Loads all platforms (default) - `--platform linux/amd64`: Loads only the specified platform<br><br>Example: `bazel run //path/to:load_target -- --platform linux/amd64`   | String | optional |  `"auto"`  |
| <a id="image_load-image"></a>image |  Image to load. Should provide ImageManifestInfo or ImageIndexInfo.   | <a href="https://bazel.build/concepts/labels">Label</a> | required |  |
| <a id="image_load-stamp"></a>stamp |  Whether to use stamping for [template expansion](/docs/templating.md). If 'enabled', uses volatile-status.txt and version.txt if present. 'auto' uses the global default setting.   | String | optional |  `"auto"`  |
| <a id="image_load-strategy"></a>strategy |  Load strategy to use.   | String | optional |  `"auto"`  |
| <a id="image_load-tag"></a>tag |  Tag to apply when loading the image. Subject to [template expansion](/docs/templating.md).   | String | optional |  `""`  |


