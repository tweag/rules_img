<!-- Generated with Stardoc: http://skydoc.bazel.build -->

Public API for container image push rules.

<a id="image_push"></a>

## image_push

<pre>
load("@rules_img//img:push.bzl", "image_push")

image_push(<a href="#image_push-name">name</a>, <a href="#image_push-build_settings">build_settings</a>, <a href="#image_push-image">image</a>, <a href="#image_push-registry">registry</a>, <a href="#image_push-repository">repository</a>, <a href="#image_push-stamp">stamp</a>, <a href="#image_push-strategy">strategy</a>, <a href="#image_push-tag">tag</a>, <a href="#image_push-tag_list">tag_list</a>)
</pre>



**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="image_push-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="image_push-build_settings"></a>build_settings |  Build settings to use for [template expansion](/docs/templating.md). Keys are setting names, values are labels to string_flag targets.   | Dictionary: String -> Label | optional |  `{}`  |
| <a id="image_push-image"></a>image |  Image to push. Should provide ImageManifestInfo or ImageIndexInfo.   | <a href="https://bazel.build/concepts/labels">Label</a> | required |  |
| <a id="image_push-registry"></a>registry |  Registry to push the image to. Subject to [template expansion](/docs/templating.md).   | String | optional |  `""`  |
| <a id="image_push-repository"></a>repository |  Repository name of the image. Subject to [template expansion](/docs/templating.md).   | String | optional |  `""`  |
| <a id="image_push-stamp"></a>stamp |  Whether to use stamping for [template expansion](/docs/templating.md). If 'enabled', uses volatile-status.txt and version.txt if present. 'auto' uses the global default setting.   | String | optional |  `"auto"`  |
| <a id="image_push-strategy"></a>strategy |  Push strategy to use.   | String | optional |  `"auto"`  |
| <a id="image_push-tag"></a>tag |  Tag of the image. Optional - can be omitted for digest-only push. Subject to [template expansion](/docs/templating.md).   | String | optional |  `""`  |
| <a id="image_push-tag_list"></a>tag_list |  List of tags for the image. Cannot be used together with 'tag'. Subject to [template expansion](/docs/templating.md).   | List of strings | optional |  `[]`  |


