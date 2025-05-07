"""Layer rule for converting existing tar files to usable layers."""

load("//bzl/img:providers.bzl", "LayerInfo")
load(":tarfiles_helper.bzl", "allow_tar_files", "calculate_layer_info", "extension_to_compression", "optimize_layer", "recompress_layer")

def _layer_from_tar_impl(ctx):
    optimize = ctx.attr.optimize or len(ctx.attr.deduplicate) > 0
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

    extra_output_groups = {}
    if not needs_rewrite:
        # here, we can simply calculate the layer info (size, digest, etc.) and return
        layer_info = calculate_layer_info(
            ctx = ctx,
            media_type = media_type,
            tar_file = ctx.file.src,
            metadata_file = metadata_file,
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
        )
    else:
        # here, we deduplicate, recompress the tar file,
        # and calculate the layer info
        layer_info = optimize_layer(
            ctx = ctx,
            media_type = media_type,
            tar_file = ctx.file.src,
            metadata_file = metadata_file,
            content_manifest = ctx.actions.declare_file(ctx.attr.name + ".content_manifest"),
            output = ctx.actions.declare_file(ctx.attr.name + output_name_extension),
            target_compression = target_compression,
        )
        extra_output_groups["content_manifest"] = layer_info.content_manifests

    return [
        DefaultInfo(
            files = depset([layer_info.blob, layer_info.metadata]),
        ),
        OutputGroupInfo(
            layer = depset([layer_info.blob]),
            metadata = depset([layer_info.metadata]),
            **extra_output_groups
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
This is useful for reducing the size of the image, but will take extra time and space to store the optimized layer.
Rewriting is also enabled by passing other layers to the `deduplicate` attribute.""",
        ),
        "deduplicate": attr.label_list(
            doc = """Optional layers or images that are known to be below this layer.
Any files included in referenced layers will not be written again.
Users are free to choose: adding a layer here adds an ordering constraint (referenced layers have to be built first), but doing so can reduce image size.""",
        ),
        "_tool": attr.label(
            executable = True,
            cfg = "exec",
            default = Label("//cmd/img"),
        ),
    },
    provides = [LayerInfo],
)
