"""Image index rule for composing multi-layer OCI images."""

load("//img/private/common:transitions.bzl", "multi_platform_image_transition", "reset_platform_transition")
load("//img/private/common:write_index_json.bzl", "write_index_json")
load("//img/private/providers:index_info.bzl", "ImageIndexInfo")
load("//img/private/providers:manifest_info.bzl", "ImageManifestInfo")
load("//img/private/providers:pull_info.bzl", "PullInfo")

def _image_index_impl(ctx):
    pull_infos = [manifest[PullInfo] for manifest in ctx.attr.manifests if PullInfo in manifest]
    pull_info = pull_infos[0] if len(pull_infos) > 0 else None
    for other in pull_infos:
        if pull_info != other:
            fail("index rule called with images that are based on different external images. This is not yet supported.")
    index_out = ctx.actions.declare_file(ctx.attr.name + "_index.json")
    write_index_json(
        ctx,
        output = index_out,
        manifests = [manifest[ImageManifestInfo] for manifest in ctx.attr.manifests],
        annotations = ctx.attr.annotations,
    )
    providers = [
        DefaultInfo(files = depset([index_out])),
        ImageIndexInfo(
            index = index_out,
            manifests = [manifest[ImageManifestInfo] for manifest in ctx.attr.manifests],
        ),
    ]
    if pull_info != None:
        providers.append(pull_info)
    return providers

image_index = rule(
    implementation = _image_index_impl,
    attrs = {
        "manifests": attr.label_list(
            providers = [ImageManifestInfo],
            doc = "List of manifests for specific platforms.",
            cfg = multi_platform_image_transition,
        ),
        "platforms": attr.label_list(
            providers = [platform_common.PlatformInfo],
        ),
        "annotations": attr.string_dict(
            doc = "Arbitrary metadata for the image index.",
        ),
        "_tool": attr.label(
            executable = True,
            cfg = "exec",
            default = Label("//cmd/img"),
        ),
    },
    cfg = reset_platform_transition,
)
