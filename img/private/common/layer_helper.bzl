"""Helper functions for working with tar files."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("//img/private/common:build.bzl", "TOOLCHAIN")
load("//img/private/providers:layer_info.bzl", "LayerInfo")

allow_tar_files = [".tar", ".tar.gz", ".tgz", ".tar.zst", ".tzst"]

extension_to_compression = {
    "tar": "none",
    "gz": "gzip",
    "tar.gz": "gzip",
    "tgz": "gzip",
    "zst": "zstd",
    "tar.zst": "zstd",
    "tzst": "zstd",
}

def compression_tuning_args(ctx, compression):
    """Compression tuning arguments for img tools based on build mode.

    Returns additional CLI arguments to tune gzip compression defaults
    according to Bazel's compilation mode. For gzip compression this
    function prefers faster, parallel compression in fastbuild, and
    smaller, single-threaded high-compression in opt. Other compression
    algorithms are left unchanged.

    Args:
        ctx: Rule context used to read `COMPILATION_MODE`.
        compression: String name of the target compression algorithm
            (e.g., "gzip", "zstd", "none").

    Returns:
        list[string]: Flat list of flags and values, suitable for
        `ctx.actions.args().add_all(...)`.
    """

    # Set compressor defaults based on compilation mode for gzip
    if compression != "gzip":
        return []

    # Start with mode-based defaults
    mode = ctx.var.get("COMPILATION_MODE", "fastbuild")
    jobs = "nproc" if mode != "opt" else "1"
    level = "-1"  # default compression
    if mode == "opt":
        level = "9"  # high compression
    elif mode == "fastbuild":
        level = "1"  # faster, lower compression

    # Apply global overrides if present as hidden attrs
    if hasattr(ctx.attr, "_compression_jobs") and ctx.attr._compression_jobs != None:
        val = ctx.attr._compression_jobs[BuildSettingInfo].value
        if val and val != "auto":
            jobs = val
    if hasattr(ctx.attr, "_compression_level") and ctx.attr._compression_level != None:
        lvl = ctx.attr._compression_level[BuildSettingInfo].value
        if lvl and lvl != "auto":
            level = lvl

    return ["--compressor-jobs", jobs, "--compression-level", level]

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
    args.add_all(compression_tuning_args(ctx, target_compression))
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
    args.add_all(compression_tuning_args(ctx, target_compression))
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
