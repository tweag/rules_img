"""Load settings rule."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("//img/private/providers:oci_layout_settings_info.bzl", "OCILayoutSettingsInfo")

def _oci_layout_settings_impl(ctx):
    return [OCILayoutSettingsInfo(
        allow_shallow_oci_layout = ctx.attr._shallow_oci_layout[BuildSettingInfo].value == "i_know_what_i_am_doing",
    )]

oci_layout_settings = rule(
    implementation = _oci_layout_settings_impl,
    attrs = {
        "_shallow_oci_layout": attr.label(
            default = Label("//img/settings:shallow_oci_layout"),
            providers = [BuildSettingInfo],
        ),
    },
)
