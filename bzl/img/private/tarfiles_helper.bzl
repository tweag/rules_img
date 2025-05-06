"""Helper functions for working with tar files."""

load("//bzl/img:providers.bzl", "LayerInfo")

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
    args.add(tar_file.path)
    args.add(metadata_file.path)
    ctx.actions.run(
        inputs = [tar_file],
        outputs = [metadata_file],
        executable = ctx.executable._tool,
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
    args.add("--format", target_compression)
    args.add("--metadata", metadata_file.path)
    args.add(tar_file.path)
    args.add(output)
    ctx.actions.run(
        inputs = [tar_file],
        outputs = [output, metadata_file],
        executable = ctx.executable._tool,
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
    args = ctx.actions.args()
    args.add("layer")
    args.add("--format", target_compression)
    args.add("--metadata", metadata_file.path)
    args.add("--import-tar", tar_file.path)
    args.add(output)
    ctx.actions.run(
        inputs = [tar_file],
        outputs = [output, metadata_file],
        executable = ctx.executable._tool,
        arguments = [args],
        mnemonic = "LayerOptimize",
    )
    return LayerInfo(
        blob = output,
        metadata = metadata_file,
        media_type = media_type,
    )
