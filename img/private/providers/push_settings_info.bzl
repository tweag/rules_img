"""Defines providers for settings of push rules."""

DOC = """\
Collection of active push settings.
"""

FIELDS = dict(
    strategy = "The strategy of the push rule. This can be one of the following: 'upload', 'cas'",
)

PushSettingsInfo = provider(
    doc = DOC,
    fields = FIELDS,
)
