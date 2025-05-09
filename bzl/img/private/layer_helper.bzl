"""Helper functions for working with tar files."""

load("//bzl/img:providers.bzl", "LayerInfo")

allow_tar_files = [".tar", ".tar.gz", ".tgz"]

extension_to_compression = {
    "tar": "none",
    "gz": "gzip",
    "tar.gz": "gzip",
    "tgz": "gzip",
}

def collect_content_manifests(ctx, direct = []):
    """Collects deduplicated files."""
    if not hasattr(ctx.attr, "deduplicate") or ctx.attr.deduplicate == None:
        return depset(direct)
    transitive = []
    for collection in ctx.attr.deduplicate:
        layer_info = collection[LayerInfo]
        if layer_info.content_manifests == None:
            continue
        transitive.append(layer_info.content_manifests)
    return depset(direct, transitive = transitive)

def collect_required_layers(ctx, direct = []):
    """Collects required_layers."""
    if not hasattr(ctx.attr, "deduplicate") or ctx.attr.deduplicate == None:
        return depset(direct)
    direct = list(direct)
    transitive = []
    for collection in ctx.attr.deduplicate:
        layer_info = collection[LayerInfo]
        direct.append(layer_info)
        if layer_info.required_layers == None:
            continue
        transitive.append(layer_info.required_layers)
    return depset(direct, transitive = transitive)

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
        content_manifests = None,
        required_layers = None,
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
        content_manifests = None,
        media_type = media_type,
        required_layers = None,
    )

def optimize_layer(*, ctx, media_type, tar_file, metadata_file, content_manifest, output, target_compression):
    """Optimizes a tar file."""
    inputs = [tar_file]
    transitive_content_manifests = []
    required_layers = None
    args = ctx.actions.args()
    args.add("layer")
    args.add("--format", target_compression)
    args.add("--metadata", metadata_file.path)
    args.add("--content-manifest", content_manifest.path)
    args.add("--import-tar", tar_file.path)
    if hasattr(ctx.attr, "deduplicate") and ctx.attr.deduplicate != None:
        required_layers = collect_required_layers(ctx)
        collections = ctx.actions.args()
        collections.set_param_file_format("multiline")
        collections.use_param_file("--deduplicate-collection=%s", use_always = True)
        content_manifests = collect_content_manifests(ctx)
        collections.add_all(content_manifests)
        collections_param_file = ctx.actions.declare_file(ctx.label.name + ".deduplicate-collection")
        ctx.actions.write(collections_param_file, collections)
        transitive_content_manifests.append(content_manifests)
        inputs.append(collections_param_file)
        args.add("--deduplicate-collection", collections_param_file)
    args.add(output)
    ctx.actions.run(
        inputs = inputs,
        outputs = [output, metadata_file, content_manifest],
        executable = ctx.executable._tool,
        arguments = [args],
        mnemonic = "LayerOptimize",
    )
    return LayerInfo(
        blob = output,
        metadata = metadata_file,
        content_manifests = depset([content_manifest], transitive = transitive_content_manifests),
        required_layers = required_layers,
        media_type = media_type,
    )
