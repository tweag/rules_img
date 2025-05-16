"""Defines providers for about pulled base images."""

DOC = """\
Information corresponding to a pulled image.
"""

FIELDS = dict(
    registries = "List of registry mirrors used to pull the image.",
    repository = "Repository name of the image.",
    tag = "Tag of the image.",
    digest = "Digest of the image.",
)

PullInfo = provider(
    doc = DOC,
    fields = FIELDS,
)
