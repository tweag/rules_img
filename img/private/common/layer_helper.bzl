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

def calculate_layer_info(*, ctx, media_type, tar_file, metadata_file, estargz, annotations = {}):
    """Calculates the layer info for a tar file.

    Args:
        ctx: Rule context.
        media_type: Media type of the layer.
        tar_file: Input tar file.
        metadata_file: Output metadata file.
        estargz: Boolean indicating whether the layer is an estargz layer.
        annotations: Dict of string annotations to add to the layer metadata.

    Returns:
        LayerInfo provider with blob, metadata, and media type.
    """
    args = ctx.actions.args()
    args.add("layer-metadata")
    args.add("--name", ctx.label)
    for key, value in annotations.items():
        args.add("--annotation", "{}={}".format(key, value))
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
        estargz = estargz,
    )

def recompress_layer(*, ctx, media_type, tar_file, metadata_file, output, target_compression, estargz, annotations):
    """Recompresses a tar file.

    Args:
        ctx: Rule context.
        media_type: Media type of the layer.
        tar_file: Input tar file.
        metadata_file: Input metadata file.
        output: Output recompressed file.
        target_compression: Target compression format.
        estargz: Boolean indicating whether the layer is an estargz layer.
        annotations: Dict of string annotations to add to the layer metadata.

    Returns:
        LayerInfo provider with recompressed blob and metadata.
    """
    args = ctx.actions.args()
    args.add("compress")
    args.add("--name", ctx.label)
    args.add("--format", target_compression)
    if estargz:
        args.add("--estargz")
    for key, value in annotations.items():
        args.add("--annotation", "{}={}".format(key, value))
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
        estargz = estargz,
    )

def optimize_layer(*, ctx, media_type, tar_file, metadata_file, output, target_compression, estargz, annotations):
    """Optimizes a tar file.

    Args:
        ctx: Rule context.
        media_type: Media type of the layer.
        tar_file: Input tar file.
        metadata_file: Input metadata file.
        output: Output optimized file.
        target_compression: Target compression format.
        estargz: Boolean indicating whether the layer is an estargz layer.
        annotations: Dict of string annotations to add to the layer metadata.

    Returns:
        LayerInfo provider with optimized blob and metadata.
    """
    inputs = [tar_file]
    args = ctx.actions.args()
    args.add("layer")
    args.add("--name", ctx.attr.name)
    args.add("--format", target_compression)
    if estargz:
        args.add("--estargz")
    for key, value in annotations.items():
        args.add("--annotation", "{}={}".format(key, value))
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
        estargz = estargz,
    )
