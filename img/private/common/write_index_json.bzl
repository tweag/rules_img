"""Utilities for writing index.json files."""

load("//img/private/common:build.bzl", "TOOLCHAIN")

def _annotation_arg(tup):
    return "{}={}".format(tup[0], tup[1])

def write_index_json(ctx, *, output, digest, manifests, config_json = None):
    """Write an index.json file for a multi-platform image.

    Args:
        ctx: Rule context.
        output: Output file to write.
        digest: Digest file to write.
        manifests: List of manifests to include in the index.
        config_json: Optional config JSON file with template expansions.
    """
    manifest_descriptors = [manifest.descriptor for manifest in manifests]
    args = ctx.actions.args()
    args.add("index")
    args.add("--digest", digest.path)
    args.add_all(manifest_descriptors, format_each = "--manifest-descriptor=%s")

    inputs = manifest_descriptors

    if config_json:
        args.add("--config-templates", config_json.path)
        inputs.append(config_json)
    else:
        args.add_all(ctx.attr.annotations.items(), map_each = _annotation_arg, format_each = "--annotation=%s")

    args.add(output.path)
    img_toolchain_info = ctx.toolchains[TOOLCHAIN].imgtoolchaininfo
    ctx.actions.run(
        outputs = [output, digest],
        inputs = inputs,
        executable = img_toolchain_info.tool_exe,
        arguments = [args],
        mnemonic = "ImageIndex",
    )
