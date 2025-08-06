"""Push rule for uploading images to a registry."""

load("//img/private/common:build.bzl", "TOOLCHAIN", "TOOLCHAINS")
load("//img/private/common:transitions.bzl", "host_platform_transition", "reset_platform_transition")
load("//img/private/providers:image_toolchain_info.bzl", "ImageToolchainInfo")
load("//img/private/providers:index_info.bzl", "ImageIndexInfo")
load("//img/private/providers:manifest_info.bzl", "ImageManifestInfo")
load("//img/private/providers:pull_info.bzl", "PullInfo")
load("//img/private/providers:push_settings_info.bzl", "PushSettingsInfo")

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

    request_json = ctx.actions.declare_file(ctx.label.name + ".json")
    ctx.actions.write(
        request_json,
        json.encode(push_request),
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

    request_metadata = ctx.actions.declare_file(ctx.label.name + "_request_metadata.json")
    ctx.actions.write(
        request_metadata,
        json.encode(push_request),
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
                # TODO: Make the default configurable.
                "IMG_CREDENTIAL_HELPER": "tweag-credential-helper",
            },
            inherited_environment = [
                "IMG_CREDENTIAL_HELPER",
                "IMG_REAPI_ENDPOINT",
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
    attrs = {
        "registry": attr.string(
            doc = "Registry to push the image to.",
        ),
        "repository": attr.string(
            doc = "Repository name of the image.",
        ),
        "tag": attr.string(
            doc = "Tag of the image. Optional - can be omitted for digest-only push.",
        ),
        "tag_list": attr.string_list(
            doc = "List of tags for the image. Cannot be used together with 'tag'.",
        ),
        "image": attr.label(
            doc = "Image to push. Should provide ImageManifestInfo or ImageIndexInfo.",
            mandatory = True,
        ),
        "strategy": attr.string(
            doc = "Push strategy to use.",
            default = "auto",
            values = ["auto", "eager", "lazy", "cas_registry", "bes"],
        ),
        "_push_settings": attr.label(
            default = Label("//img/private/settings:push"),
            providers = [PushSettingsInfo],
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
