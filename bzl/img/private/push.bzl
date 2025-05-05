"""Push rule for uploading images to a registry."""

load("//bzl/img:providers.bzl", "ImageIndexInfo", "ImageManifestInfo", "PullInfo")

def _transition_to_host_platform(_settings, _attr):
    return {
        "//command_line_option:platforms": ["@@bazel_tools//tools:host_platform"],
        "//command_line_option:extra_execution_platforms": ["@@bazel_tools//tools:host_platform"],
    }

_transition_to_host = transition(
    implementation = _transition_to_host_platform,
    inputs = [],
    outputs = [
        "//command_line_option:platforms",
        "//command_line_option:extra_execution_platforms",
    ],
)

def _to_rlocation_path(ctx, file):
    if file.short_path.startswith("../"):
        return file.short_path[3:]
    return ctx.workspace_name + "/" + file.short_path

def _encode_manifest(ctx, manifest_info):
    layers = []
    for layer in manifest_info.layers:
        blob_path = _to_rlocation_path(ctx, layer.blob) if layer.blob != None else ""
        layers.append(dict(
            metadata = _to_rlocation_path(ctx, layer.metadata),
            blob_path = blob_path,
        ))
    return dict(
        manifest = _to_rlocation_path(ctx, manifest_info.manifest),
        config = _to_rlocation_path(ctx, manifest_info.config),
        layers = layers,
        missing_blobs = manifest_info.missing_blobs,
    )

def _layer_runfiles_for_manifest(manifest_info):
    """Returns the runfiles for a manifest."""
    runfiles = []
    for layer in manifest_info.layers:
        if layer.blob != None:
            runfiles.append(layer.blob)
        if layer.metadata != None:
            runfiles.append(layer.metadata)
    return runfiles

def _push_impl(ctx):
    """Implementation of the push rule."""

    pusher = ctx.actions.declare_file(ctx.label.name + ".exe")
    ctx.actions.symlink(
        output = pusher,
        target_file = ctx.executable._tool,
        is_executable = True,
    )
    pull_info = ctx.attr.image[PullInfo] if PullInfo in ctx.attr.image else None
    manifest_info = ctx.attr.image[ImageManifestInfo] if ImageManifestInfo in ctx.attr.image else None
    index_info = ctx.attr.image[ImageIndexInfo] if ImageIndexInfo in ctx.attr.image else None
    if manifest_info == None and index_info == None:
        fail("image must provide ImageManifestInfo or ImageIndexInfo")
    if manifest_info != None and index_info != None:
        fail("image must provide either ImageManifestInfo or ImageIndexInfo, not both")

    direct_runfiles = []
    push_request = dict(
        registry = ctx.attr.registry,
        repository = ctx.attr.repository,
        tag = ctx.attr.tag,
    )
    if pull_info != None:
        push_request["original_registries"] = pull_info.registries
        push_request["original_repository"] = pull_info.repository
        push_request["original_tag"] = pull_info.tag
        push_request["original_digest"] = pull_info.digest
    if manifest_info != None:
        push_request["manifest"] = _encode_manifest(ctx, manifest_info)
        direct_runfiles.append(manifest_info.manifest)
        direct_runfiles.append(manifest_info.config)
        direct_runfiles.extend(_layer_runfiles_for_manifest(manifest_info))
    if index_info != None:
        direct_runfiles.append(index_info.index)
        push_request["index"] = dict(
            index = _to_rlocation_path(ctx, index_info.index),
            manifests = [
                _encode_manifest(ctx, manifest)
                for manifest in index_info.manifests
            ],
        )
        for manifest in index_info.manifests:
            direct_runfiles.append(manifest.manifest)
            direct_runfiles.append(manifest.config)
            direct_runfiles.extend(_layer_runfiles_for_manifest(manifest))

    request_json = ctx.actions.declare_file(ctx.label.name + ".json")
    ctx.actions.write(
        request_json,
        json.encode(push_request),
    )
    return [
        DefaultInfo(
            files = depset([request_json]),
            executable = pusher,
            runfiles = ctx.runfiles(
                files = direct_runfiles,
                root_symlinks = {
                    "push_request.json": request_json,
                },
            ),
        ),
    ]

push = rule(
    implementation = _push_impl,
    attrs = {
        "registry": attr.string(
            doc = "Registry to push the image to.",
        ),
        "repository": attr.string(
            doc = "Repository name of the image.",
        ),
        "tag": attr.string(
            doc = "Tag of the image.",
        ),
        "image": attr.label(
            doc = "Image to push. Should provide ImageManifestInfo or ImageIndexInfo.",
            mandatory = True,
        ),
        "_tool": attr.label(
            executable = True,
            cfg = _transition_to_host,
            default = Label("//cmd/img"),
        ),
    },
    executable = True,
)
