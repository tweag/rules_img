"""Provider definitions"""

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
        "layers": "Layers of the image as depset of Files in postorder.",
        "missing_blobs": """List or depset of hex-encoded sha256 hashes.
Used to convey information lost during shallow image pulling, where the base image layers are referenced, but never materialized.""",
    },
)

ImageIndexInfo = provider(
    doc = "Information corresponding to a (multi-platform) image index.",
    fields = {
        "manifests": "ImageManifestInfo of the images.",
    },
)
