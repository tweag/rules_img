"""Image index rule for composing multi-layer OCI images."""

load("//img/private/common:build.bzl", "TOOLCHAIN", "TOOLCHAINS")
load("//img/private/common:transitions.bzl", "multi_platform_image_transition", "reset_platform_transition")
load("//img/private/common:write_index_json.bzl", "write_index_json")
load("//img/private/providers:index_info.bzl", "ImageIndexInfo")
load("//img/private/providers:manifest_info.bzl", "ImageManifestInfo")
load("//img/private/providers:pull_info.bzl", "PullInfo")

def _build_oci_layout(ctx, index_out, manifests):
    """Build the OCI layout for a multi-platform image.

    Args:
        ctx: Rule context.
        index_out: The index file.
        manifests: List of ImageManifestInfo providers.

    Returns:
        The OCI layout directory (tree artifact).
    """
    oci_layout_dir = ctx.actions.declare_directory(ctx.label.name + "_oci_layout")

    args = ctx.actions.args()
    args.add("oci-layout")
    args.add("--index", index_out.path)
    args.add("--output", oci_layout_dir.path)

    inputs = [index_out]

    # Add manifest and config files for each platform
    for manifest in manifests:
        args.add("--manifest-path", manifest.manifest.path)
        args.add("--config-path", manifest.config.path)
        inputs.append(manifest.manifest)
        inputs.append(manifest.config)

        # Add layers with metadata=blob mapping
        for layer in manifest.layers:
            if layer.blob != None:
                args.add("--layer", "{}={}".format(layer.metadata.path, layer.blob.path))
                inputs.append(layer.metadata)
                inputs.append(layer.blob)

    img_toolchain_info = ctx.toolchains[TOOLCHAIN].imgtoolchaininfo
    ctx.actions.run(
        inputs = inputs,
        outputs = [oci_layout_dir],
        executable = img_toolchain_info.tool_exe,
        arguments = [args],
        mnemonic = "OCILayoutIndex",
    )

    return oci_layout_dir

def _image_index_impl(ctx):
    pull_infos = [manifest[PullInfo] for manifest in ctx.attr.manifests if PullInfo in manifest]
    pull_info = pull_infos[0] if len(pull_infos) > 0 else None
    for other in pull_infos:
        if pull_info != other:
            fail("index rule called with images that are based on different external images. This is not yet supported.")
    index_out = ctx.actions.declare_file(ctx.attr.name + "_index.json")
    manifests = [manifest[ImageManifestInfo] for manifest in ctx.attr.manifests]
    write_index_json(
        ctx,
        output = index_out,
        manifests = manifests,
    )
    providers = [
        DefaultInfo(files = depset([index_out])),
        OutputGroupInfo(
            oci_layout = depset([_build_oci_layout(ctx, index_out, manifests)]),
        ),
        ImageIndexInfo(
            index = index_out,
            manifests = manifests,
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
            doc = "(Optional) list of target platforms to build the manifest for. Uses a split transition. If specified, the 'manifests' attribute should contain exactly one manifest.",
        ),
        "annotations": attr.string_dict(
            doc = "Arbitrary metadata for the image index.",
        ),
    },
    toolchains = TOOLCHAINS,
    cfg = reset_platform_transition,
)
