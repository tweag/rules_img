"""Provider for oci layout settings."""

DOC = """\
Collection of active oci layout settings.
"""

FIELDS = dict(
    allow_shallow_oci_layout = "Whether to allow shallow oci layout. This is a non-standard layout where some blobs are missing.",
)

OCILayoutSettingsInfo = provider(
    doc = DOC,
    fields = FIELDS,
)
