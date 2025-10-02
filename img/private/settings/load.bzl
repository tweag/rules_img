"""Load settings rule."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("//img/private/providers:load_settings_info.bzl", "LoadSettingsInfo")

def _load_settings_impl(ctx):
    return [LoadSettingsInfo(
        strategy = ctx.attr._load_strategy[BuildSettingInfo].value,
        daemon = ctx.attr._load_daemon[BuildSettingInfo].value,
        docker_loader_path = ctx.attr._docker_loader_path[BuildSettingInfo].value,
        remote_cache = ctx.attr._remote_cache[BuildSettingInfo].value,
        credential_helper = ctx.attr._credential_helper[BuildSettingInfo].value,
    )]

load_settings = rule(
    implementation = _load_settings_impl,
    attrs = {
        "_load_strategy": attr.label(
            default = Label("//img/settings:load_strategy"),
            providers = [BuildSettingInfo],
        ),
        "_load_daemon": attr.label(
            default = Label("//img/settings:load_daemon"),
            providers = [BuildSettingInfo],
        ),
        "_remote_cache": attr.label(
            default = Label("//img/settings:remote_cache"),
            providers = [BuildSettingInfo],
        ),
        "_docker_loader_path": attr.label(
            default = Label("//img/settings:docker_loader_path"),
            providers = [BuildSettingInfo],
        ),
        "_credential_helper": attr.label(
            default = Label("//img/settings:credential_helper"),
            providers = [BuildSettingInfo],
        ),
    },
)
