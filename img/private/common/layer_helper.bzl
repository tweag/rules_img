"""Helper functions for working with tar files."""

load("//img/private/common:build.bzl", "TOOLCHAIN")
load("//img/private/providers:layer_info.bzl", "LayerInfo")

allow_tar_files = [".tar", ".tar.gz", ".tgz"]

extension_to_compression = {
    "tar": "none",
    "gz": "gzip",
    "tar.gz": "gzip",
    "tgz": "gzip",
}

def calculate_layer_info(*, ctx, media_type, tar_file, metadata_file):
    """Calculates the layer info for a tar file."""
    args = ctx.actions.args()
    args.add("layer-metadata")
    args.add("--name", ctx.label)
    args.add(tar_file.path)
    args.add(metadata_file.path)
    img_toolchain_info = ctx.toolchains[TOOLCHAIN].imgtoolchaininfo
    ctx.actions.run(
        inputs = [tar_file],
        outputs = [metadata_file],
        executable = img_toolchain_info.tool_exe,
        arguments = [args],
        mnemonic = "LayerMetadata",
    )
    return LayerInfo(
        blob = tar_file,
        metadata = metadata_file,
        media_type = media_type,
    )

def recompress_layer(*, ctx, media_type, tar_file, metadata_file, output, target_compression):
    """Recompresses a tar file."""
    args = ctx.actions.args()
    args.add("compress")
    args.add("--name", ctx.label)
    args.add("--format", target_compression)
    args.add("--metadata", metadata_file.path)
    args.add(tar_file.path)
    args.add(output)
    img_toolchain_info = ctx.toolchains[TOOLCHAIN].imgtoolchaininfo
    ctx.actions.run(
        inputs = [tar_file],
        outputs = [output, metadata_file],
        executable = img_toolchain_info.tool_exe,
        arguments = [args],
        mnemonic = "LayerCompress",
    )
    return LayerInfo(
        blob = output,
        metadata = metadata_file,
        media_type = media_type,
    )

def optimize_layer(*, ctx, media_type, tar_file, metadata_file, output, target_compression):
    """Optimizes a tar file."""
    inputs = [tar_file]
    args = ctx.actions.args()
    args.add("layer")
    args.add("--name", ctx.attr.name)
    args.add("--format", target_compression)
    args.add("--metadata", metadata_file.path)
    args.add("--import-tar", tar_file.path)
    args.add(output)
    img_toolchain_info = ctx.toolchains[TOOLCHAIN].imgtoolchaininfo
    ctx.actions.run(
        inputs = depset(inputs),
        outputs = [output, metadata_file],
        executable = img_toolchain_info.tool_exe,
        arguments = [args],
        mnemonic = "LayerOptimize",
    )
    return LayerInfo(
        blob = output,
        metadata = metadata_file,
        media_type = media_type,
    )
