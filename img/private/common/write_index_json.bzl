"""Utilities for writing index.json files."""

load("//img/private/common:build.bzl", "TOOLCHAIN")

def _annotation_arg(tup):
    return "{}={}".format(tup[0], tup[1])

def write_index_json(ctx, *, output, manifests, _annotations):
    """Write an index.json file for a multi-platform image.

    Args:
        ctx: Rule context.
        output: Output file to write.
        manifests: List of manifests to include in the index.
        _annotations: Unused parameter (annotations come from ctx.attr.annotations).
    """
    manifest_descriptors = [manifest.descriptor for manifest in manifests]
    args = ctx.actions.args()
    args.add("index")
    args.add_all(manifest_descriptors, format_each = "--manifest-descriptor=%s")
    args.add_all(ctx.attr.annotations.items(), map_each = _annotation_arg, format_each = "--annotation=%s")
    args.add(output.path)
    img_toolchain_info = ctx.toolchains[TOOLCHAIN].imgtoolchaininfo
    ctx.actions.run(
        outputs = [output],
        inputs = manifest_descriptors,
        executable = img_toolchain_info.tool_exe,
        arguments = [args],
        mnemonic = "ImageIndex",
    )
