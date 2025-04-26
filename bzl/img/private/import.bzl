load("//bzl/img:providers.bzl", "ImageIndexInfo", "ImageManifestInfo")

def _image_index_import_impl(ctx):
    decoded = json.decode(ctx.attr.encoded)
    for (i, manifest_info) in enumerate(decoded):
        manifest = ctx.files.manifests[i]
        config = ctx.files.configs[i]
        decoded[i]["manifest"] = manifest
        decoded[i]["config"] = config

    return ImageIndexInfo(
        manifests = [
            ImageManifestInfo(**manifest_info)
            for manifest_info in decoded
        ],
    )

image_index_import = rule(
    implementation = _image_index_import_impl,
    attrs = {
        "encoded": attr.string(),
        "manifests": attr.label_list(
            allow_files = True,
        ),
        "configs": attr.label_list(
            allow_files = True,
        ),
    },
)

def _image_manifest_import_impl(ctx):
    decoded = json.decode(ctx.attr.encoded)
    decoded["manifest"] = ctx.file.manifest
    decoded["config"] = ctx.file.config
    return [ImageManifestInfo(**decoded)]

image_manifest_import = rule(
    implementation = _image_manifest_import_impl,
    attrs = {
        "encoded": attr.string(),
        "manifest": attr.label(allow_single_file = True),
        "config": attr.label(allow_single_file = True),
    },
)

def import_from_json(name, encoded):
    decoded = json.decode(encoded)
    if type(decoded) == type([]):
        # image index
        image_index_import(
            name = name,
            encoded = encoded,
            manifests = [manifest_info["manifest"] for manifest_info in decoded],
            configs = [manifest_info["config"] for manifest_info in decoded],
        )
        return

    # image manifest
    image_manifest_import(
        name = name,
        encoded = encoded,
        manifest = decoded["manifest"],
        config = decoded["config"],
    )
