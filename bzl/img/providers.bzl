"""Provider definitions"""

LayerInfo = provider(
    doc = "Information corresponding to a single layer.",
    fields = {
        "blob": "File containing the raw layer or None (for shallow base images).",
        "metadata": """File containing metadata of the layer as JSON object with the keys
 - diff_id: The diff ID of the layer as a string. Example: sha256:1234567890abcdef.
 - mediaType: The media type of the layer as a string. Example: application/vnd.oci.image.layer.v1.tar+gzip.
 - digest: The sha256 hash of the layer as a string. Example: sha256:1234567890abcdef.
 - size: The size of the layer in bytes as an int.
""",
        "content_manifests": "Depset of File containing binary content manifest or None. This is used by downstream layers to deduplicate contents.",
        "media_type": "The media type of the layer as a string. Example: application/vnd.oci.image.layer.v1.tar+gzip.",
    },
)

ImageManifestInfo = provider(
    doc = "Information corresponding to a single image manifest for one platform.",
    fields = {
        "base_image": "ImageManifestInfo of the base image (or None).",
        "manifest": "File containing the raw image manifest (application/vnd.oci.image.index.v1+json).",
        "config": "File containing the raw image config (application/vnd.oci.image.config.v1+json).",
        "structured_config": "(Partial) image config with values known in the analysis phase.",
        "architecture": "The CPU architecture this image runs on.",
        "os": "The operating system this image runs on.",
        "platform": "Dict containing additional runtime requirements of the image.",
        "layers": "Layers of the image as list of LayerInfo.",
        "missing_blobs": """List of hex-encoded sha256 hashes.
Used to convey information lost during shallow image pulling, where the base image layers are referenced, but never materialized.""",
    },
)

ImageIndexInfo = provider(
    doc = "Information corresponding to a (multi-platform) image index.",
    fields = {
        "index": "File containing the raw image index (application/vnd.oci.image.index.v1+json).",
        "manifests": "ImageManifestInfo of the images.",
    },
)

PullInfo = provider(
    doc = "Information corresponding to a pulled image.",
    fields = {
        "registries": "List of registry mirrors used to pull the image.",
        "repository": "Repository name of the image.",
        "tag": "Tag of the image.",
        "digest": "Digest of the image.",
    },
)
