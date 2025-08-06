<!-- Generated with Stardoc: http://skydoc.bazel.build -->

Public API for container image layer rules.

<a id="image_layer"></a>

## image_layer

<pre>
load("@rules_img//img:layer.bzl", "image_layer")

image_layer(<a href="#image_layer-name">name</a>, <a href="#image_layer-srcs">srcs</a>, <a href="#image_layer-annotations">annotations</a>, <a href="#image_layer-compress">compress</a>, <a href="#image_layer-estargz">estargz</a>, <a href="#image_layer-symlinks">symlinks</a>)
</pre>



**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="image_layer-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="image_layer-srcs"></a>srcs |  Files (including regular files, executables, and TreeArtifacts) that should be added to the layer.   | Dictionary: String -> Label | optional |  `{}`  |
| <a id="image_layer-annotations"></a>annotations |  Annotations to add to the layer metadata as key-value pairs.   | <a href="https://bazel.build/rules/lib/dict">Dictionary: String -> String</a> | optional |  `{}`  |
| <a id="image_layer-compress"></a>compress |  Compression algorithm to use. If set to 'auto', uses the global default compression setting.   | String | optional |  `"auto"`  |
| <a id="image_layer-estargz"></a>estargz |  Whether to use estargz format. If set to 'auto', uses the global default estargz setting. When enabled, the layer will be optimized for lazy pulling and will be compatible with the estargz format.   | String | optional |  `"auto"`  |
| <a id="image_layer-symlinks"></a>symlinks |  Symlinks that should be added to the layer.   | <a href="https://bazel.build/rules/lib/dict">Dictionary: String -> String</a> | optional |  `{}`  |


<a id="layer_from_tar"></a>

## layer_from_tar

<pre>
load("@rules_img//img:layer.bzl", "layer_from_tar")

layer_from_tar(<a href="#layer_from_tar-name">name</a>, <a href="#layer_from_tar-src">src</a>, <a href="#layer_from_tar-annotations">annotations</a>, <a href="#layer_from_tar-compress">compress</a>, <a href="#layer_from_tar-estargz">estargz</a>, <a href="#layer_from_tar-optimize">optimize</a>)
</pre>



**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="layer_from_tar-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="layer_from_tar-src"></a>src |  The tar file to convert into a layer. Must be a valid tar file (optionally compressed).   | <a href="https://bazel.build/concepts/labels">Label</a> | required |  |
| <a id="layer_from_tar-annotations"></a>annotations |  Annotations to add to the layer metadata as key-value pairs.   | <a href="https://bazel.build/rules/lib/dict">Dictionary: String -> String</a> | optional |  `{}`  |
| <a id="layer_from_tar-compress"></a>compress |  Compression algorithm to use. If set to 'auto', uses the global default compression setting.   | String | optional |  `"auto"`  |
| <a id="layer_from_tar-estargz"></a>estargz |  Whether to use estargz format. If set to 'auto', uses the global default estargz setting. When enabled, the layer will be optimized for lazy pulling and will be compatible with the estargz format.   | String | optional |  `"auto"`  |
| <a id="layer_from_tar-optimize"></a>optimize |  If set, rewrites the tar file to deduplicate it's contents. This is useful for reducing the size of the image, but will take extra time and space to store the optimized layer.   | Boolean | optional |  `False`  |


