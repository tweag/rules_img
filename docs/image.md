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

image_manifest(<a href="#image_manifest-name">name</a>, <a href="#image_manifest-architecture">architecture</a>, <a href="#image_manifest-base">base</a>, <a href="#image_manifest-cmd">cmd</a>, <a href="#image_manifest-config_fragment">config_fragment</a>, <a href="#image_manifest-entrypoint">entrypoint</a>, <a href="#image_manifest-env">env</a>, <a href="#image_manifest-labels">labels</a>, <a href="#image_manifest-layers">layers</a>, <a href="#image_manifest-os">os</a>,
               <a href="#image_manifest-platform">platform</a>, <a href="#image_manifest-stop_signal">stop_signal</a>, <a href="#image_manifest-user">user</a>, <a href="#image_manifest-working_dir">working_dir</a>)
</pre>



**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="image_manifest-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="image_manifest-architecture"></a>architecture |  The CPU architecture this image runs on.   | String | optional |  `""`  |
| <a id="image_manifest-base"></a>base |  Base image to inherit layers from. Should provide ImageManifestInfo or ImageIndexInfo.   | <a href="https://bazel.build/concepts/labels">Label</a> | optional |  `None`  |
| <a id="image_manifest-cmd"></a>cmd |  Default arguments to the entrypoint of the container. These values act as defaults and may be replaced by any specified when creating a container. If an Entrypoint value is not specified, then the first entry of the Cmd array SHOULD be interpreted as the executable to run.   | List of strings | optional |  `[]`  |
| <a id="image_manifest-config_fragment"></a>config_fragment |  Optional JSON file containing a partial image config, which will be used as a base for the final image config.   | <a href="https://bazel.build/concepts/labels">Label</a> | optional |  `None`  |
| <a id="image_manifest-entrypoint"></a>entrypoint |  A list of arguments to use as the command to execute when the container starts. These values act as defaults and may be replaced by an entrypoint specified when creating a container.   | List of strings | optional |  `[]`  |
| <a id="image_manifest-env"></a>env |  Default environment variables to set when starting a container based on this image.   | <a href="https://bazel.build/rules/lib/dict">Dictionary: String -> String</a> | optional |  `{}`  |
| <a id="image_manifest-labels"></a>labels |  This field contains arbitrary metadata for the container.   | <a href="https://bazel.build/rules/lib/dict">Dictionary: String -> String</a> | optional |  `{}`  |
| <a id="image_manifest-layers"></a>layers |  Layers to include in the image.   | <a href="https://bazel.build/concepts/labels">List of labels</a> | optional |  `[]`  |
| <a id="image_manifest-os"></a>os |  The operating system this image runs on.   | String | optional |  `""`  |
| <a id="image_manifest-platform"></a>platform |  Dict containing additional runtime requirements of the image.   | <a href="https://bazel.build/rules/lib/dict">Dictionary: String -> String</a> | optional |  `{}`  |
| <a id="image_manifest-stop_signal"></a>stop_signal |  This field contains the system call signal that will be sent to the container to exit. The signal can be a signal name in the format SIGNAME, for instance SIGKILL or SIGRTMIN+3.   | String | optional |  `""`  |
| <a id="image_manifest-user"></a>user |  The username or UID which is a platform-specific structure that allows specific control over which user the process run as. This acts as a default value to use when the value is not specified when creating a container.   | String | optional |  `""`  |
| <a id="image_manifest-working_dir"></a>working_dir |  Sets the current working directory of the entrypoint process in the container. This value acts as a default and may be replaced by a working directory specified when creating a container.   | String | optional |  `""`  |


