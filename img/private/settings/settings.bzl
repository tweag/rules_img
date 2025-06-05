load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("//img/private/providers:push_settings_info.bzl", "PushSettingsInfo")

def _push_settings_impl(ctx):
    strategy = ctx.attr._push_strategy[BuildSettingInfo].value

    return [PushSettingsInfo(
        strategy = strategy,
    )]

push_settings = rule(
    implementation = _push_settings_impl,
    attrs = {
        "_push_strategy": attr.label(
            default = Label("//img/settings:push_strategy"),
            providers = [BuildSettingInfo],
        ),
    },
)
