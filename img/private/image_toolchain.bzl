"""Implementation of the image_toolchain rule."""

load("//img/private/common:transitions.bzl", "reset_platform_transition", "toolchain_transition")
load("//img/private/providers:image_toolchain_info.bzl", "ImageToolchainInfo")

DOC = """\
Defines an image builder toolchain.

The image build tool can natively target any platform,
so it only has exec platform constraints.

See https://bazel.build/extending/toolchains#defining-toolchains.
"""

ATTRS = dict(
    tool_exe = attr.label(
        doc = "An image build tool executable.",
        default = Label("//cmd/img"),
        allow_single_file = True,
        cfg = toolchain_transition,
    ),
    exec_platform = attr.label(
        doc = "The (optional) platform to use when building the tool from source.",
        mandatory = False,
        providers = [platform_common.PlatformInfo],
    ),
)

TOOLCHAIN_TYPE = str(Label("//img:toolchain_type"))

def _image_toolchain_impl(ctx):
    image_toolchain_info = ImageToolchainInfo(
        tool_exe = ctx.file.tool_exe,
    )
    toolchain_info = platform_common.ToolchainInfo(
        imgtoolchaininfo = image_toolchain_info,
    )

    return [toolchain_info]

image_toolchain = rule(
    implementation = _image_toolchain_impl,
    attrs = ATTRS,
    doc = DOC,
    cfg = reset_platform_transition,
)
