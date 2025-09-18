"""Push rule for uploading images to a registry."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("//img/private:stamp.bzl", "expand_or_write")
load("//img/private/common:build.bzl", "TOOLCHAIN", "TOOLCHAINS")
load("//img/private/common:transitions.bzl", "host_platform_transition", "reset_platform_transition")
load("//img/private/providers:image_toolchain_info.bzl", "ImageToolchainInfo")
load("//img/private/providers:index_info.bzl", "ImageIndexInfo")
load("//img/private/providers:manifest_info.bzl", "ImageManifestInfo")
load("//img/private/providers:pull_info.bzl", "PullInfo")
load("//img/private/providers:push_settings_info.bzl", "PushSettingsInfo")
load("//img/private/providers:stamp_setting_info.bzl", "StampSettingInfo")

def _encode_manifest(manifest_info, path_prefix = ""):
    layers = []
    for i, layer in enumerate(manifest_info.layers):
        blob_path = "{path_prefix}/layer/{i}".format(path_prefix = path_prefix, i = i) if layer.blob != None else ""
        blob_path = blob_path.removeprefix("/")
        metadata = "{path_prefix}/metadata/{i}".format(path_prefix = path_prefix, i = i)
        metadata = metadata.removeprefix("/")
        layers.append(dict(
            metadata = metadata,
            blob_path = blob_path,
        ))
    manifest = "{}/manifest.json".format(path_prefix)
    manifest = manifest.removeprefix("/")
    config = "{}/config.json".format(path_prefix)
    config = config.removeprefix("/")
    return dict(
        manifest = manifest,
        config = config,
        layers = layers,
        missing_blobs = manifest_info.missing_blobs,
    )

def _encode_manifest_metadata(manifest_info):
    manifest = manifest_info.manifest.path
    return dict(
        manifest = manifest,
        missing_blobs = manifest_info.missing_blobs,
    )

def _layer_root_symlinks_for_manifest(manifest_info, index = None):
    base_path = "layer" if index == None else "manifests/{}/layer".format(index)
    return {
        "{base}/{layer_index}".format(base = base_path, layer_index = layer_index): layer.blob
        for (layer_index, layer) in enumerate(manifest_info.layers)
        if layer.blob != None
    }

def _metadata_symlinks_for_manifest(manifest_info, index = None):
    base_path = "metadata" if index == None else "manifests/{}/metadata".format(index)
    return {
        "{base}/{layer_index}".format(base = base_path, layer_index = layer_index): layer.metadata
        for (layer_index, layer) in enumerate(manifest_info.layers)
        if layer.metadata != None
    }

def _root_symlinks_for_manifest(manifest_info, index = None, *, include_layers):
    base_path = "" if index == None else "manifests/{}/".format(index)
    root_symlinks = {
        "{base}manifest.json".format(base = base_path): manifest_info.manifest,
        "{base}config.json".format(base = base_path): manifest_info.config,
    }
    if include_layers:
        root_symlinks.update(_layer_root_symlinks_for_manifest(manifest_info, index))
        root_symlinks.update(_metadata_symlinks_for_manifest(manifest_info, index))
    return root_symlinks

def _root_symlinks(index_info, manifest_info, *, include_layers):
    root_symlinks = {}
    if index_info != None:
        root_symlinks["index.json"] = index_info.index
        for i, manifest in enumerate(index_info.manifests):
            root_symlinks.update(_root_symlinks_for_manifest(manifest, index = i, include_layers = include_layers))
    if manifest_info != None:
        root_symlinks.update(_root_symlinks_for_manifest(manifest_info, include_layers = include_layers))
    return root_symlinks

def _push_strategy(ctx):
    """Determine the push strategy to use based on the settings."""
    push_settings = ctx.attr._push_settings[PushSettingsInfo]
    strategy = ctx.attr.strategy
    if strategy == "auto":
        strategy = push_settings.strategy
    return strategy

def _target_info(ctx):
    pull_info = ctx.attr.image[PullInfo] if PullInfo in ctx.attr.image else None
    if pull_info == None:
        return {}
    return dict(
        original_registries = pull_info.registries,
        original_repository = pull_info.repository,
        original_tag = pull_info.tag,
        original_digest = pull_info.digest,
    )

def _get_tags(ctx):
    """Get the list of tags from the context, validating mutual exclusivity."""
    if ctx.attr.tag and ctx.attr.tag_list:
        fail("Cannot specify both 'tag' and 'tag_list' attributes")

    tags = []
    if ctx.attr.tag:
        tags = [ctx.attr.tag]
    elif ctx.attr.tag_list:
        tags = ctx.attr.tag_list

    # Empty list is allowed for digest-only push
    return tags

def _image_push_upload_impl(ctx):
    """Regular image push rule (bazel run target)."""

    pusher = ctx.actions.declare_file(ctx.label.name + ".exe")
    img_toolchain_info = ctx.attr._tool[0][ImageToolchainInfo]
    ctx.actions.symlink(
        output = pusher,
        target_file = img_toolchain_info.tool_exe,
        is_executable = True,
    )
    manifest_info = ctx.attr.image[ImageManifestInfo] if ImageManifestInfo in ctx.attr.image else None
    index_info = ctx.attr.image[ImageIndexInfo] if ImageIndexInfo in ctx.attr.image else None
    if manifest_info == None and index_info == None:
        fail("image must provide ImageManifestInfo or ImageIndexInfo")
    if manifest_info != None and index_info != None:
        fail("image must provide either ImageManifestInfo or ImageIndexInfo, not both")

    root_symlinks = _root_symlinks(index_info, manifest_info, include_layers = True)

    # Create push request
    push_request = dict(
        command = "push",
        registry = ctx.attr.registry,
        repository = ctx.attr.repository,
        tags = _get_tags(ctx),
    )
    push_request.update(_target_info(ctx))
    if manifest_info != None:
        push_request["manifest"] = _encode_manifest(manifest_info)
    if index_info != None:
        push_request["index"] = dict(
            index = "index.json",
            manifests = [
                _encode_manifest(manifest, "manifests/{}".format(i))
                for i, manifest in enumerate(index_info.manifests)
            ],
        )

    # Either expand templates or write directly
    request_json = expand_or_write(
        ctx = ctx,
        request = push_request,
        output_name = ctx.label.name + ".json",
        kind = "push",
    )
    root_symlinks["dispatch.json"] = request_json
    return [
        DefaultInfo(
            files = depset([request_json]),
            executable = pusher,
            runfiles = ctx.runfiles(
                root_symlinks = root_symlinks,
            ),
        ),
    ]

def _image_push_cas_impl(ctx):
    """CAS push rule (bazel run target)."""
    pusher = ctx.actions.declare_file(ctx.label.name + ".exe")
    img_toolchain_info = ctx.attr._tool[0][ImageToolchainInfo]
    ctx.actions.symlink(
        output = pusher,
        target_file = img_toolchain_info.tool_exe,
        is_executable = True,
    )

    inputs = []
    manifest_info = ctx.attr.image[ImageManifestInfo] if ImageManifestInfo in ctx.attr.image else None
    index_info = ctx.attr.image[ImageIndexInfo] if ImageIndexInfo in ctx.attr.image else None
    if manifest_info == None and index_info == None:
        fail("image must provide ImageManifestInfo or ImageIndexInfo")
    if manifest_info != None and index_info != None:
        fail("image must provide either ImageManifestInfo or ImageIndexInfo, not both")

    root_symlinks = _root_symlinks(index_info, manifest_info, include_layers = False)

    # Create push request
    push_request = dict(
        command = "push-metadata",
        strategy = _push_strategy(ctx),
        registry = ctx.attr.registry,
        repository = ctx.attr.repository,
        tags = _get_tags(ctx),
    )
    push_request.update(_target_info(ctx))

    if manifest_info != None:
        push_request["manifest"] = _encode_manifest_metadata(manifest_info)
        inputs.append(manifest_info.manifest)
    if index_info != None:
        push_request["index"] = dict(
            index = index_info.index.path,
            manifests = [
                _encode_manifest_metadata(manifest)
                for manifest in index_info.manifests
            ],
        )
        inputs.append(index_info.index)
        inputs.extend([manifest.manifest for manifest in index_info.manifests])

    # Either expand templates or write directly
    request_metadata = expand_or_write(
        ctx = ctx,
        request = push_request,
        output_name = ctx.label.name + "_request_metadata.json",
        kind = "push",
    )
    inputs.append(request_metadata)

    metadata_out = ctx.actions.declare_file(ctx.label.name + ".json")

    img_toolchain_info = ctx.toolchains[TOOLCHAIN].imgtoolchaininfo
    ctx.actions.run(
        inputs = inputs,
        outputs = [metadata_out],
        executable = img_toolchain_info.tool_exe,
        arguments = ["push-metadata", "--from-file", request_metadata.path, metadata_out.path],
        mnemonic = "PushMetadata",
    )
    root_symlinks["dispatch.json"] = metadata_out
    return [
        DefaultInfo(
            files = depset([metadata_out]),
            executable = pusher,
            runfiles = ctx.runfiles(
                root_symlinks = root_symlinks,
            ),
        ),
        RunEnvironmentInfo(
            environment = {
                "IMG_REAPI_ENDPOINT": ctx.attr._push_settings[PushSettingsInfo].remote_cache,
                "IMG_CREDENTIAL_HELPER": ctx.attr._push_settings[PushSettingsInfo].credential_helper,
            },
            inherited_environment = [
                "IMG_REAPI_ENDPOINT",
                "IMG_CREDENTIAL_HELPER",
            ],
        ),
    ]

def _image_push_impl(ctx):
    """Implementation of the push rule."""
    strategy = _push_strategy(ctx)
    if strategy == "eager":
        return _image_push_upload_impl(ctx)
    elif strategy in ["lazy", "cas_registry", "bes"]:
        return _image_push_cas_impl(ctx)
    else:
        fail("Unknown push strategy: {}".format(strategy))

image_push = rule(
    implementation = _image_push_impl,
    doc = """Pushes container images to a registry.

This rule creates an executable target that uploads OCI images to container registries.
It supports multiple push strategies optimized for different use cases, from simple
uploads to advanced content-addressable storage integration.

Key features:
- **Multiple push strategies**: Choose between eager, lazy, CAS-based, or BES-integrated pushing
- **Template expansion**: Dynamic registry, repository, and tag values using build settings
- **Stamping support**: Include build information in image tags
- **Incremental uploads**: Skip blobs that already exist in the registry

The rule produces an executable that can be run with `bazel run`.

Example:

```python
load("@rules_img//img:push.bzl", "image_push")

# Simple push to Docker Hub
image_push(
    name = "push_app",
    image = ":my_app",
    registry = "index.docker.io",
    repository = "myorg/myapp",
    tag = "latest",
)

# Push multi-platform image with multiple tags
image_push(
    name = "push_multiarch",
    image = ":my_app_index",  # References an image_index
    registry = "gcr.io",
    repository = "my-project/my-app",
    tag_list = ["latest", "v1.0.0"],
)

# Dynamic push with build settings
image_push(
    name = "push_dynamic",
    image = ":my_app",
    registry = "{{.REGISTRY}}",
    repository = "{{.PROJECT}}/my-app",
    tag = "{{.VERSION}}",
    build_settings = {
        "REGISTRY": "//settings:registry",
        "PROJECT": "//settings:project",
        "VERSION": "//settings:version",
    },
)

# Push with stamping for unique tags
image_push(
    name = "push_stamped",
    image = ":my_app",
    registry = "index.docker.io",
    repository = "myorg/myapp",
    tag = "latest-{{.BUILD_TIMESTAMP}}",
    stamp = "enabled",
)

# Digest-only push (no tag)
image_push(
    name = "push_by_digest",
    image = ":my_app",
    registry = "gcr.io",
    repository = "my-project/my-app",
    # No tag specified - will push by digest only
)
```

Push strategies:
- **`eager`**: Materializes all layers next to push binary. Simple, correct, but may be inefficient.
- **`lazy`**: Layers are not stored locally. Missing layers are streamed from Bazel's remote cache.
- **`cas_registry`**: Uses content-addressable storage for extreme efficiency. Requires
  CAS-enabled infrastructure.
- **`bes`**: Image is pushed as side-effect of Build Event Stream upload. No "bazel run" command needed.
  Requires Build Event Service integration.

See [push strategies documentation](/docs/push-strategies.md) for detailed comparisons.

Runtime usage:
```bash
# Push to registry
bazel run //path/to:push_app

# The push command will output the image digest
```
""",
    attrs = {
        "registry": attr.string(
            doc = """Registry URL to push the image to.

Common registries:
- Docker Hub: `index.docker.io`
- Google Container Registry: `gcr.io` or `us.gcr.io`
- GitHub Container Registry: `ghcr.io`
- Amazon ECR: `123456789.dkr.ecr.us-east-1.amazonaws.com`

Subject to [template expansion](/docs/templating.md).
""",
        ),
        "repository": attr.string(
            doc = """Repository path within the registry.

Subject to [template expansion](/docs/templating.md).
""",
        ),
        "tag": attr.string(
            doc = """Tag to apply to the pushed image.

Optional - if omitted, the image is pushed by digest only.

Subject to [template expansion](/docs/templating.md).
""",
        ),
        "tag_list": attr.string_list(
            doc = """List of tags to apply to the pushed image.

Useful for applying multiple tags in a single push:

```python
tag_list = ["latest", "v1.0.0", "stable"]
```

Cannot be used together with `tag`. Each tag is subject to [template expansion](/docs/templating.md).
""",
        ),
        "image": attr.label(
            doc = "Image to push. Should provide ImageManifestInfo or ImageIndexInfo.",
            mandatory = True,
        ),
        "strategy": attr.string(
            doc = """Push strategy to use.

See [push strategies documentation](/docs/push-strategies.md) for detailed information.
""",
            default = "auto",
            values = ["auto", "eager", "lazy", "cas_registry", "bes"],
        ),
        "build_settings": attr.string_keyed_label_dict(
            doc = """Build settings for template expansion.

Maps template variable names to string_flag targets. These values can be used in
registry, repository, and tag attributes using `{{.VARIABLE_NAME}}` syntax (Go template).

Example:
```python
build_settings = {
    "REGISTRY": "//settings:docker_registry",
    "VERSION": "//settings:app_version",
}
```

See [template expansion](/docs/templating.md) for more details.
""",
            providers = [BuildSettingInfo],
        ),
        "stamp": attr.string(
            doc = """Enable build stamping for template expansion.

Controls whether to include volatile build information:
- **`auto`** (default): Uses the global stamping configuration
- **`enabled`**: Always include stamp information (BUILD_TIMESTAMP, BUILD_USER, etc.) if Bazel's "--stamp" flag is set
- **`disabled`**: Never include stamp information

See [template expansion](/docs/templating.md) for available stamp variables.
""",
            default = "auto",
            values = ["auto", "enabled", "disabled"],
        ),
        "_push_settings": attr.label(
            default = Label("//img/private/settings:push"),
            providers = [PushSettingsInfo],
        ),
        "_stamp_settings": attr.label(
            default = Label("//img/private/settings:stamp"),
            providers = [StampSettingInfo],
        ),
        "_tool": attr.label(
            cfg = host_platform_transition,
            default = Label("//img:resolved_toolchain"),
        ),
    },
    executable = True,
    cfg = reset_platform_transition,
    toolchains = TOOLCHAINS,
)
