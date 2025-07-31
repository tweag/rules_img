"""Layer rule for converting existing tar files to usable layers."""

load("//img/private/common:build.bzl", "TOOLCHAINS")
load("//img/private/common:layer_helper.bzl", "allow_tar_files", "calculate_layer_info", "extension_to_compression", "optimize_layer", "recompress_layer")
load("//img/private/providers:layer_info.bzl", "LayerInfo")

def _layer_from_tar_impl(ctx):
    optimize = ctx.attr.optimize
    source_compression = extension_to_compression[ctx.file.src.extension]
    target_compression = source_compression if ctx.attr.compress == "auto" else ctx.attr.compress
    needs_recompression = source_compression != target_compression
    needs_rewrite = needs_recompression or optimize

    media_type = "application/vnd.oci.image.layer.v1.tar"
    metadata_file = ctx.actions.declare_file("{}_metadata.json".format(ctx.attr.name))
    if target_compression != "none":
        media_type += "+{}".format(target_compression)
    if target_compression == "gzip":
        output_name_extension = ".tgz"
    elif target_compression == "none":
        output_name_extension = ".tar"
    else:
        fail("Unsupported compression algorithm: {}".format(target_compression))

    if not needs_rewrite:
        # here, we can simply calculate the layer info (size, digest, etc.) and return
        layer_info = calculate_layer_info(
            ctx = ctx,
            media_type = media_type,
            tar_file = ctx.file.src,
            metadata_file = metadata_file,
            estargz = ctx.attr.estargz,
            annotations = ctx.attr.annotations,
        )
    elif not optimize:
        # here, we recompress the tar file and calculate the layer info
        layer_info = recompress_layer(
            ctx = ctx,
            media_type = media_type,
            tar_file = ctx.file.src,
            metadata_file = metadata_file,
            output = ctx.actions.declare_file(ctx.attr.name + output_name_extension),
            target_compression = target_compression,
            estargz = ctx.attr.estargz,
            annotations = ctx.attr.annotations,
        )
    else:
        # here, we optimize, recompress the tar file,
        # and calculate the layer info
        layer_info = optimize_layer(
            ctx = ctx,
            media_type = media_type,
            tar_file = ctx.file.src,
            metadata_file = metadata_file,
            output = ctx.actions.declare_file(ctx.attr.name + output_name_extension),
            target_compression = target_compression,
            estargz = ctx.attr.estargz,
            annotations = ctx.attr.annotations,
        )

    return [
        DefaultInfo(
            files = depset([layer_info.blob, layer_info.metadata]),
        ),
        OutputGroupInfo(
            layer = depset([layer_info.blob]),
            metadata = depset([layer_info.metadata]),
        ),
        layer_info,
    ]

layer_from_tar = rule(
    implementation = _layer_from_tar_impl,
    attrs = {
        "src": attr.label(
            mandatory = True,
            allow_single_file = allow_tar_files,
        ),
        "compress": attr.string(
            default = "auto",
            values = ["auto", "none", "gzip"],
            doc = """Compression algorithm to use when creating the tar file. If this is set to `auto`, the algorithm will be chosen based on the file extension.
If the file extension is `.tar` or the compression is none, no compression will be used. This may lead to the tar file being rewritten if the output compression is different from the input compression.""",
        ),
        "optimize": attr.bool(
            doc = """If set, rewrites the tar file to deduplicate it's contents.
This is useful for reducing the size of the image, but will take extra time and space to store the optimized layer.""",
        ),
        "estargz": attr.bool(
            default = False,
            doc = """If set, the layer will be treated as an estargz layer.
This means that the layer will be optimized for lazy pulling and will be compatible with the estargz format.""",
        ),
        "annotations": attr.string_dict(
            default = {},
            doc = """Annotations to add to the layer metadata as key-value pairs.""",
        ),
    },
    toolchains = TOOLCHAINS,
    provides = [LayerInfo],
)
