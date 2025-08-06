"""This file defines the stamp build setting for Bazel."""

load("//img/private/providers:stamp_setting_info.bzl", "StampSettingInfo")

def _stamp_build_setting_impl(ctx):
    return StampSettingInfo(
        bazel_setting = ctx.attr.bazel_setting,
        user_preference = ctx.attr.user_preference,
    )

stamp_build_setting = rule(
    implementation = _stamp_build_setting_impl,
    attrs = {
        "bazel_setting": attr.bool(
            doc = "The value of the stamp build flag",
            mandatory = True,
        ),
        "user_preference": attr.bool(
            doc = "Whether the user prefers to use volatile-status.txt and version.txt if present",
            mandatory = True,
        ),
    },
)
