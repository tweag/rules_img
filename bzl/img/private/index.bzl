"""Image index rule for composing multi-layer OCI images."""

load("//bzl/img:providers.bzl", "ImageIndexInfo", "ImageManifestInfo", "PullInfo")
load("//bzl/img/private:transitions.bzl", "multi_platform_image_transition", "reset_platform_transition")
load("//bzl/img/private:write_index_json.bzl", "write_index_json")

def _index_impl(ctx):
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

index = rule(
    implementation = _index_impl,
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
