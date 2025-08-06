"""Defines providers for about stamping."""

DOC = """\
Information on stamping configuration.
"""

FIELDS = dict(
    bazel_setting = "bool: Whether or not the `--stamp` flag was enabled",
    user_preference = "bool: Whether volatile-status.txt and version.txt should be used if present",
)

StampSettingInfo = provider(
    doc = DOC,
    fields = FIELDS,
)
