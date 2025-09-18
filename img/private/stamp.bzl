"""Shared stamping utilities for Bazel rules."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("//img/private/common:build.bzl", "TOOLCHAIN")
load("//img/private/providers:stamp_setting_info.bzl", "StampSettingInfo")

def get_build_settings(ctx):
    """Extract build settings values from the context.

    Args:
        ctx: The rule context

    Returns:
        A dictionary mapping setting names to their values
    """
    settings = {}
    for setting_name, setting_label in ctx.attr.build_settings.items():
        settings[setting_name] = setting_label[BuildSettingInfo].value
    return settings

def should_stamp(ctx):
    """Get the stamp configuration from the context.

    Args:
        ctx: The rule context

    Returns:
        A struct containing stamp, can_stamp, and want_stamp boolean fields
    """
    stamp_settings = ctx.attr._stamp_settings[StampSettingInfo]
    can_stamp = stamp_settings.bazel_setting
    global_user_preference = stamp_settings.user_preference
    target_stamp = ctx.attr.stamp

    want_stamp = False
    if target_stamp == "disabled":
        want_stamp = False
    elif target_stamp == "enabled":
        want_stamp = True
    elif target_stamp == "auto":
        want_stamp = global_user_preference
    return struct(
        stamp = can_stamp and want_stamp,
        can_stamp = can_stamp,
        want_stamp = want_stamp,
    )

def expand_or_write(ctx, request, output_name, kind):
    """Either expand templates or write JSON directly based on build_settings.

    Args:
        ctx: The rule context
        request: The request dictionary (push_request, load_request, etc.)
        output_name: The name for the output file
        kind: The kind of template expansion (e.g., "push", "load")

    Returns:
        The File object for the final JSON
    """
    build_settings = get_build_settings(ctx)
    stamp_settings = should_stamp(ctx)

    if build_settings or stamp_settings.want_stamp:
        # Add build settings to the request for template expansion
        request["build_settings"] = build_settings

        # Write the template JSON
        template_name = output_name.replace(".json", "_template.json")
        template_json = ctx.actions.declare_file(template_name)
        ctx.actions.write(
            template_json,
            json.encode(request),
        )

        # Run expand-template to create the final JSON
        final_json = ctx.actions.declare_file(output_name)

        # Build arguments for expand-template
        args = []
        inputs = [template_json]

        # Add stamp files if stamping is enabled
        if stamp_settings.stamp:
            if ctx.version_file:
                args.extend(["--stamp", ctx.version_file.path])
                inputs.append(ctx.version_file)
            if ctx.info_file:
                args.extend(["--stamp", ctx.info_file.path])
                inputs.append(ctx.info_file)

        args.extend([template_json.path, final_json.path])

        img_toolchain_info = ctx.toolchains[TOOLCHAIN].imgtoolchaininfo
        ctx.actions.run(
            inputs = inputs,
            outputs = [final_json],
            executable = img_toolchain_info.tool_exe,
            arguments = ["expand-template", "--kind", kind] + args,
            mnemonic = "ExpandTemplate",
        )
        return final_json
    else:
        # No templates to expand, create JSON directly
        final_json = ctx.actions.declare_file(output_name)
        ctx.actions.write(
            final_json,
            json.encode(request),
        )
        return final_json
