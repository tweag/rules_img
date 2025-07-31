<!-- Generated with Stardoc: http://skydoc.bazel.build -->

Public API for container image push rules.

<a id="image_push"></a>

## image_push

<pre>
load("@rules_img//img:push.bzl", "image_push")

image_push(<a href="#image_push-name">name</a>, <a href="#image_push-image">image</a>, <a href="#image_push-registry">registry</a>, <a href="#image_push-repository">repository</a>, <a href="#image_push-strategy">strategy</a>, <a href="#image_push-tag">tag</a>)
</pre>



**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="image_push-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="image_push-image"></a>image |  Image to push. Should provide ImageManifestInfo or ImageIndexInfo.   | <a href="https://bazel.build/concepts/labels">Label</a> | required |  |
| <a id="image_push-registry"></a>registry |  Registry to push the image to.   | String | optional |  `""`  |
| <a id="image_push-repository"></a>repository |  Repository name of the image.   | String | optional |  `""`  |
| <a id="image_push-strategy"></a>strategy |  Push strategy to use.   | String | optional |  `"auto"`  |
| <a id="image_push-tag"></a>tag |  Tag of the image.   | String | optional |  `""`  |


