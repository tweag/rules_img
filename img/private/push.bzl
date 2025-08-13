"""Push rule for uploading images to a registry."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
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

def _get_build_settings(ctx):
    """Extract build settings values from the context."""
    settings = {}
    for setting_name, setting_label in ctx.attr.build_settings.items():
        settings[setting_name] = setting_label[BuildSettingInfo].value
    return settings

def _should_stamp(ctx):
    """Get the stamp configuration from the context."""
    stamp_settings = ctx.attr._stamp_settings[StampSettingInfo]
    can_stamp = stamp_settings.bazel_setting
    global_user_preference = stamp_settings.user_preference
    target_stamp = ctx.attr.stamp

    want_stamp = False
    if target_stamp == "disabled":
        want_stamp = False
    elif target_stamp == "enabled":
        want_stamp = True
    elif target_stamp == "auto":
        want_stamp = global_user_preference
    return struct(
        stamp = can_stamp and want_stamp,
        can_stamp = can_stamp,
        want_stamp = want_stamp,
    )

def _expand_or_write(ctx, push_request, output_name):
    """Either expand templates or write JSON directly based on build_settings.

    Args:
        ctx: The rule context
        push_request: The push request dictionary
        output_name: The name for the output file

    Returns:
        The File object for the final JSON
    """
    build_settings = _get_build_settings(ctx)
    stamp_settings = _should_stamp(ctx)

    if build_settings or stamp_settings.want_stamp:
        # Add build settings to the request for template expansion
        push_request["build_settings"] = build_settings

        # Write the template JSON
        template_name = output_name.replace(".json", "_template.json")
        template_json = ctx.actions.declare_file(template_name)
        ctx.actions.write(
            template_json,
            json.encode(push_request),
        )

        # Run expand-template to create the final JSON
        final_json = ctx.actions.declare_file(output_name)

        # Build arguments for expand-template
        args = []
        inputs = [template_json]

        # Add stamp files if stamping is enabled
        if stamp_settings.stamp:
            if ctx.version_file:
                args.extend(["--stamp", ctx.version_file.path])
                inputs.append(ctx.version_file)
            if ctx.info_file:
                args.extend(["--stamp", ctx.info_file.path])
                inputs.append(ctx.info_file)

        args.extend([template_json.path, final_json.path])

        img_toolchain_info = ctx.toolchains[TOOLCHAIN].imgtoolchaininfo
        ctx.actions.run(
            inputs = inputs,
            outputs = [final_json],
            executable = img_toolchain_info.tool_exe,
            arguments = ["expand-template"] + args,
            mnemonic = "ExpandTemplate",
        )
        return final_json
    else:
        # No templates to expand, create JSON directly
        final_json = ctx.actions.declare_file(output_name)
        ctx.actions.write(
            final_json,
            json.encode(push_request),
        )
        return final_json

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
    request_json = _expand_or_write(ctx, push_request, ctx.label.name + ".json")
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
    request_metadata = _expand_or_write(ctx, push_request, ctx.label.name + "_request_metadata.json")
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
    attrs = {
        "registry": attr.string(
            doc = "Registry to push the image to. Subject to [template expansion](/docs/templating.md).",
        ),
        "repository": attr.string(
            doc = "Repository name of the image. Subject to [template expansion](/docs/templating.md).",
        ),
        "tag": attr.string(
            doc = "Tag of the image. Optional - can be omitted for digest-only push. Subject to [template expansion](/docs/templating.md).",
        ),
        "tag_list": attr.string_list(
            doc = "List of tags for the image. Cannot be used together with 'tag'. Subject to [template expansion](/docs/templating.md).",
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
        "build_settings": attr.string_keyed_label_dict(
            doc = "Build settings to use for [template expansion](/docs/templating.md). Keys are setting names, values are labels to string_flag targets.",
            providers = [BuildSettingInfo],
        ),
        "stamp": attr.string(
            doc = "Whether to use stamping for [template expansion](/docs/templating.md). If 'enabled', uses volatile-status.txt and version.txt if present. 'auto' uses the global default setting.",
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
