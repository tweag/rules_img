<!-- Generated with Stardoc: http://skydoc.bazel.build -->

Rules to build container images from layers.

Use `image_manifest` to create a single-platform container image,
and `image_index` to compose a multi-platform container image index.

<a id="image_index"></a>

## image_index

<pre>
load("@rules_img//img:image.bzl", "image_index")

image_index(<a href="#image_index-name">name</a>, <a href="#image_index-annotations">annotations</a>, <a href="#image_index-manifests">manifests</a>, <a href="#image_index-platforms">platforms</a>)
</pre>



**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="image_index-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="image_index-annotations"></a>annotations |  Arbitrary metadata for the image index.   | <a href="https://bazel.build/rules/lib/dict">Dictionary: String -> String</a> | optional |  `{}`  |
| <a id="image_index-manifests"></a>manifests |  List of manifests for specific platforms.   | <a href="https://bazel.build/concepts/labels">List of labels</a> | optional |  `[]`  |
| <a id="image_index-platforms"></a>platforms |  -   | <a href="https://bazel.build/concepts/labels">List of labels</a> | optional |  `[]`  |


<a id="image_manifest"></a>

## image_manifest

<pre>
load("@rules_img//img:image.bzl", "image_manifest")

image_manifest(<a href="#image_manifest-name">name</a>, <a href="#image_manifest-architecture">architecture</a>, <a href="#image_manifest-base">base</a>, <a href="#image_manifest-config_fragment">config_fragment</a>, <a href="#image_manifest-layers">layers</a>, <a href="#image_manifest-os">os</a>, <a href="#image_manifest-platform">platform</a>)
</pre>



**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="image_manifest-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="image_manifest-architecture"></a>architecture |  The CPU architecture this image runs on.   | String | optional |  `""`  |
| <a id="image_manifest-base"></a>base |  Base image to inherit layers from. Should provide ImageManifestInfo or ImageIndexInfo.   | <a href="https://bazel.build/concepts/labels">Label</a> | optional |  `None`  |
| <a id="image_manifest-config_fragment"></a>config_fragment |  Optional JSON file containing a partial image config, which will be used as a base for the final image config.   | <a href="https://bazel.build/concepts/labels">Label</a> | optional |  `None`  |
| <a id="image_manifest-layers"></a>layers |  Layers to include in the image.   | <a href="https://bazel.build/concepts/labels">List of labels</a> | optional |  `[]`  |
| <a id="image_manifest-os"></a>os |  The operating system this image runs on.   | String | optional |  `""`  |
| <a id="image_manifest-platform"></a>platform |  Dict containing additional runtime requirements of the image.   | <a href="https://bazel.build/rules/lib/dict">Dictionary: String -> String</a> | optional |  `{}`  |


