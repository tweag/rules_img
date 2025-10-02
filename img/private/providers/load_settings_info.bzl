"""Provider for load settings."""

DOC = """\
Collection of active load settings.
"""

FIELDS = dict(
    strategy = "The default load strategy to use",
    daemon = "The daemon to target by default",
    docker_loader_path = "Path to the docker loader binary to use. Uses $LOADER env var if not set.",
    remote_cache = "Bazel remote cache to use for the push rule as part of the lazy push strategy. Uses the same format as Bazel's --remote_cache flag. Uses $IMG_REAPI_ENDPOINT env var if not set.",
    credential_helper = "Credential helper to use for the push rule. This can be the same as Bazel's credential helper. Uses $IMG_CREDENTIAL_HELPER env var or tools/credential-helper if not set.",
)

LoadSettingsInfo = provider(
    doc = DOC,
    fields = FIELDS,
)
