"""Defines providers for a container image builder toolchain."""

DOC = """\
Information about how to invoke the container image builder tool.
"""

FIELDS = dict(
    tool_exe = "The builder executable (File).",
)

ImageToolchainInfo = provider(
    doc = DOC,
    fields = FIELDS,
)
