"""Push rule for uploading images to a registry."""

load("//img/private/common:transitions.bzl", "host_platform_transition", "reset_platform_transition")
load("//img/private/common:write_index_json.bzl", "write_index_json")
load("//img/private/providers:image_toolchain_info.bzl", "ImageToolchainInfo")
load("//img/private/providers:index_info.bzl", "ImageIndexInfo")
load("//img/private/providers:manifest_info.bzl", "ImageManifestInfo")
load("//img/private/providers:pull_info.bzl", "PullInfo")

def _encode_manifest(ctx, manifest_info, path_prefix = ""):
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

def _layer_root_symlinks_for_manifest(manifest_info, index = None):
    base_path = "layer" if index == None else "manifest/{}/layer".format(index)
    return {
        "{base}/{layer_index}".format(base = base_path, layer_index = layer_index): layer.blob
        for (layer_index, layer) in enumerate(manifest_info.layers)
        if layer.blob != None
    }

def _metadata_symlinks_for_manifest(manifest_info, index = None):
    base_path = "metadata" if index == None else "manifest/{}/metadata".format(index)
    return {
        "{base}/{layer_index}".format(base = base_path, layer_index = layer_index): layer.metadata
        for (layer_index, layer) in enumerate(manifest_info.layers)
        if layer.metadata != None
    }

def _root_symlinks_for_manifest(manifest_info, index = None):
    base_path = "" if index == None else "manifest/{}/".format(index)
    root_symlinks = {
        "{base}manifest.json".format(base = base_path): manifest_info.manifest,
        "{base}config.json".format(base = base_path): manifest_info.config,
    }
    root_symlinks.update(_layer_root_symlinks_for_manifest(manifest_info, index))
    root_symlinks.update(_metadata_symlinks_for_manifest(manifest_info, index))
    return root_symlinks

def _image_push_impl(ctx):
    """Implementation of the push rule."""

    pusher = ctx.actions.declare_file(ctx.label.name + ".exe")
    img_toolchain_info = ctx.attr._tool[0][ImageToolchainInfo]
    ctx.actions.symlink(
        output = pusher,
        target_file = img_toolchain_info.tool_exe,
        is_executable = True,
    )
    pull_info = ctx.attr.image[PullInfo] if PullInfo in ctx.attr.image else None
    manifest_info = ctx.attr.image[ImageManifestInfo] if ImageManifestInfo in ctx.attr.image else None
    index_info = ctx.attr.image[ImageIndexInfo] if ImageIndexInfo in ctx.attr.image else None
    if manifest_info == None and index_info == None:
        fail("image must provide ImageManifestInfo or ImageIndexInfo")
    if manifest_info != None and index_info != None:
        fail("image must provide either ImageManifestInfo or ImageIndexInfo, not both")

    root_symlinks = {}
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
        index_json = ctx.attr.declare_file(ctx.attr.name + "_index.json")
        write_index_json(
            ctx,
            output = index_json,
            manifests = [manifest_info],
            annotations = {},
        )
        root_symlinks["index.json"] = index_json
        root_symlinks.update(_root_symlinks_for_manifest(manifest_info))
        push_request["manifest"] = _encode_manifest(ctx, manifest_info)
    if index_info != None:
        root_symlinks["index.json"] = index_info.index
        push_request["index"] = dict(
            index = "index.json",
            manifests = [
                _encode_manifest(ctx, manifest, "manifest/{}".format(i))
                for i, manifest in enumerate(index_info.manifests)
            ],
        )
        for i, manifest in enumerate(index_info.manifests):
            root_symlinks.update(_root_symlinks_for_manifest(manifest, index = i))

    request_json = ctx.actions.declare_file(ctx.label.name + ".json")
    ctx.actions.write(
        request_json,
        json.encode(push_request),
    )
    root_symlinks["push_request.json"] = request_json
    return [
        DefaultInfo(
            files = depset([request_json]),
            executable = pusher,
            runfiles = ctx.runfiles(
                root_symlinks = root_symlinks,
            ),
        ),
    ]

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
            doc = "Tag of the image.",
        ),
        "image": attr.label(
            doc = "Image to push. Should provide ImageManifestInfo or ImageIndexInfo.",
            mandatory = True,
        ),
        "_tool": attr.label(
            cfg = host_platform_transition,
            default = Label("//img:resolved_toolchain"),
        ),
    },
    executable = True,
    cfg = reset_platform_transition,
)
