"""Image index rule for composing multi-layer OCI images."""

load("//bzl/img:providers.bzl", "ImageIndexInfo", "ImageManifestInfo", "PullInfo")

def _annotation_arg(tup):
    return "{}={}".format(tup[0], tup[1])

def _index_impl(ctx):
    inputs = []
    manifest_descriptors = [manifest[ImageManifestInfo].descriptor for manifest in ctx.attr.manifests]
    pull_infos = [manifest[PullInfo] for manifest in ctx.attr.manifests if PullInfo in manifest]
    pull_info = pull_infos[0] if len(pull_infos) > 0 else None
    for other in pull_infos:
        if pull_info != other:
            fail("index rule called with images that are based on different external images. This is not yet supported.")
    args = ctx.actions.args()
    args.add("index")
    args.add_all(manifest_descriptors, format_each = "--manifest-descriptor=%s")
    args.add_all(ctx.attr.annotations.items(), map_each = _annotation_arg, format_each = "--annotation=%s")
    index_out = ctx.actions.declare_file(ctx.attr.name + "_index.json")
    args.add(index_out.path)
    ctx.actions.run(
        outputs = [index_out],
        inputs = manifest_descriptors,
        executable = ctx.executable._tool,
        arguments = [args],
        mnemonic = "ImageIndex",
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
)
