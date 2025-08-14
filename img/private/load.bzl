"""Load rule for importing images into a container daemon."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("//img/private/common:build.bzl", "TOOLCHAIN", "TOOLCHAINS")
load("//img/private/common:transitions.bzl", "host_platform_transition", "reset_platform_transition")
load("//img/private/providers:image_toolchain_info.bzl", "ImageToolchainInfo")
load("//img/private/providers:index_info.bzl", "ImageIndexInfo")
load("//img/private/providers:load_settings_info.bzl", "LoadSettingsInfo")
load("//img/private/providers:manifest_info.bzl", "ImageManifestInfo")
load("//img/private/providers:pull_info.bzl", "PullInfo")
load("//img/private/providers:stamp_setting_info.bzl", "StampSettingInfo")

def _layer_root_symlinks_for_manifest(manifest_info, index = None):
    base_path = "layer" if index == None else "manifests/{}/layer".format(index)
    return {
        "{base}/{layer_index}".format(base = base_path, layer_index = layer_index): layer.blob
        for (layer_index, layer) in enumerate(manifest_info.layers)
        if layer.blob != None
    }

def _root_symlinks_for_manifest(manifest_info, index = None, *, include_layers):
    base_path = "" if index == None else "manifests/{}/".format(index)
    root_symlinks = {
        "{base}manifest.json".format(base = base_path): manifest_info.manifest,
        "{base}config.json".format(base = base_path): manifest_info.config,
    }
    if include_layers:
        root_symlinks.update(_layer_root_symlinks_for_manifest(manifest_info, index))
    return root_symlinks

def _root_symlinks(index_info, manifest_info, *, include_layers):
    root_symlinks = {}
    if index_info != None:
        root_symlinks["index.json"] = index_info.index
        for i, manifest in enumerate(index_info.manifests):
            root_symlinks.update(_root_symlinks_for_manifest(manifest, index = i, include_layers = include_layers))
    if manifest_info != None:
        root_symlinks.update(_root_symlinks_for_manifest(manifest_info, include_layers = include_layers))
    return root_symlinks

def _get_build_settings(ctx):
    """Extract build settings values from the context."""
    settings = {}
    for setting_name, setting_label in ctx.attr.build_settings.items():
        settings[setting_name] = setting_label[BuildSettingInfo].value
    return settings

def _should_stamp(ctx):
    """Get the stamp configuration from the context."""
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

def _expand_or_write(ctx, load_request, output_name):
    """Either expand templates or write JSON directly based on build_settings.

    Args:
        ctx: The rule context
        load_request: The load request dictionary
        output_name: The name for the output file

    Returns:
        The File object for the final JSON
    """
    build_settings = _get_build_settings(ctx)
    stamp_settings = _should_stamp(ctx)

    if build_settings or stamp_settings.want_stamp:
        # Add build settings to the request for template expansion
        load_request["build_settings"] = build_settings

        # Write the template JSON
        template_name = output_name.replace(".json", "_template.json")
        template_json = ctx.actions.declare_file(template_name)
        ctx.actions.write(
            template_json,
            json.encode(load_request),
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
            arguments = ["expand-template"] + args,
            mnemonic = "ExpandTemplate",
        )
        return final_json
    else:
        # No templates to expand, create JSON directly
        final_json = ctx.actions.declare_file(output_name)
        ctx.actions.write(
            final_json,
            json.encode(load_request),
        )
        return final_json

def _load_strategy(ctx):
    """Determine the load strategy to use based on the settings."""
    load_settings = ctx.attr._load_settings[LoadSettingsInfo]
    strategy = ctx.attr.strategy
    if strategy == "auto":
        strategy = load_settings.strategy
    return strategy

def _daemon(ctx):
    """Determine the daemon to target based on the settings."""
    load_settings = ctx.attr._load_settings[LoadSettingsInfo]
    daemon = ctx.attr.daemon
    if daemon == "auto":
        daemon = load_settings.daemon
    return daemon

def _image_load_impl(ctx):
    """Implementation of the load rule."""
    loader = ctx.actions.declare_file(ctx.label.name + ".exe")
    img_toolchain_info = ctx.attr._tool[0][ImageToolchainInfo]
    ctx.actions.symlink(
        output = loader,
        target_file = img_toolchain_info.tool_exe,
        is_executable = True,
    )

    manifest_info = ctx.attr.image[ImageManifestInfo] if ImageManifestInfo in ctx.attr.image else None
    index_info = ctx.attr.image[ImageIndexInfo] if ImageIndexInfo in ctx.attr.image else None
    if manifest_info == None and index_info == None:
        fail("image must provide ImageManifestInfo or ImageIndexInfo")
    if manifest_info != None and index_info != None:
        fail("image must provide either ImageManifestInfo or ImageIndexInfo, not both")

    # Determine strategy
    strategy = _load_strategy(ctx)
    daemon = _daemon(ctx)
    include_layers = (strategy == "eager")

    root_symlinks = _root_symlinks(index_info, manifest_info, include_layers = include_layers)

    # Create load request
    load_request = dict(
        command = "load",
        daemon = daemon,
        strategy = strategy,
    )

    # Add tag if provided
    if ctx.attr.tag:
        load_request["tag"] = ctx.attr.tag

    # Determine the image type
    if manifest_info != None:
        manifest_req = dict(
            manifest = "manifest.json",
            config = "config.json",
            layers = [
                "layer/{}".format(i)
                for i, layer in enumerate(manifest_info.layers)
                if layer.blob != None
            ],
            missing_blobs = manifest_info.missing_blobs,
        )

        # Add PullInfo if the image has pull information (from a base image)
        # This is optional - images built entirely locally won't have PullInfo
        if PullInfo in ctx.attr.image:
            pull_info = ctx.attr.image[PullInfo]
            manifest_req["original_registries"] = pull_info.registries
            manifest_req["original_repository"] = pull_info.repository

        load_request["manifest"] = manifest_req
    if index_info != None:
        manifests = []
        for i, manifest in enumerate(index_info.manifests):
            manifest_req = dict(
                manifest = "manifests/{}/manifest.json".format(i),
                config = "manifests/{}/config.json".format(i),
                layers = [
                    "manifests/{}/layer/{}".format(i, j)
                    for j, layer in enumerate(manifest.layers)
                    if layer.blob != None
                ],
                missing_blobs = manifest.missing_blobs,
            )

            # Add PullInfo if available on the index level
            if PullInfo in ctx.attr.image:
                pull_info = ctx.attr.image[PullInfo]
                manifest_req["original_registries"] = pull_info.registries
                manifest_req["original_repository"] = pull_info.repository

            manifests.append(manifest_req)

        load_request["index"] = dict(
            index = "index.json",
            manifests = manifests,
        )

    # Either expand templates or write directly
    request_json = _expand_or_write(ctx, load_request, ctx.label.name + ".json")
    root_symlinks["dispatch.json"] = request_json

    outputs = [request_json]

    return [
        DefaultInfo(
            files = depset(outputs),
            executable = loader,
            runfiles = ctx.runfiles(
                root_symlinks = root_symlinks,
            ),
        ),
        RunEnvironmentInfo(
            environment = {
                "IMG_REAPI_ENDPOINT": ctx.attr._load_settings[LoadSettingsInfo].remote_cache,
                "IMG_CREDENTIAL_HELPER": ctx.attr._load_settings[LoadSettingsInfo].credential_helper,
            },
            inherited_environment = [
                "IMG_REAPI_ENDPOINT",
                "IMG_CREDENTIAL_HELPER",
            ],
        ),
    ]

image_load = rule(
    implementation = _image_load_impl,
    doc = """Loads container images into a local daemon (Docker or containerd).

This rule creates an executable target that imports OCI images into your local
container runtime. It supports both Docker and containerd, with intelligent
detection of the best loading method for optimal performance.

Key features:
- **Incremental loading**: Skips blobs that already exist in the daemon
- **Multi-platform support**: Can load entire image indexes or specific platforms
- **Direct containerd integration**: Bypasses Docker for faster imports when possible
- **Platform filtering**: Use `--platform` flag at runtime to select specific platforms

The rule produces an executable that can be run with `bazel run`.

Example:

```python
load("@rules_img//img:load.bzl", "image_load")

# Load a single-platform image
image_load(
    name = "load_app",
    image = ":my_app",  # References an image_manifest
    tag = "my-app:latest",
)

# Load a multi-platform image
image_load(
    name = "load_multiarch",
    image = ":my_app_index",  # References an image_index
    tag = "my-app:latest",
    daemon = "containerd",  # Explicitly use containerd
)

# Load with dynamic tagging
image_load(
    name = "load_dynamic",
    image = ":my_app",
    tag = "my-app:{{.BUILD_USER}}",  # Template expansion
    build_settings = {
        "BUILD_USER": "//settings:username",
    },
)
```

Runtime usage:
```bash
# Load all platforms
bazel run //path/to:load_app

# Load specific platform only
bazel run //path/to:load_multiarch -- --platform linux/arm64
```

Performance notes:
- When Docker uses containerd storage (Docker 23.0+), images are loaded directly
  into containerd for better performance if the containerd socket is accessible.
- For older Docker versions, falls back to `docker load` which requires building
  a tar file (slower and limited to single-platform images)
- The `--platform` flag filters which platforms are loaded from multi-platform images
""",
    attrs = {
        "image": attr.label(
            doc = "Image to load. Should provide ImageManifestInfo or ImageIndexInfo.",
            mandatory = True,
        ),
        "daemon": attr.string(
            doc = """Container daemon to use for loading the image.

Available options:
- **`auto`** (default): Uses the global default setting (usually `docker`)
- **`containerd`**: Loads directly into containerd namespace. Supports multi-platform images
  and incremental loading.
- **`docker`**: Loads via Docker daemon. When Docker uses containerd storage (23.0+),
  loads directly into containerd. Otherwise falls back to `docker load` command which
  is slower and limited to single-platform images.

The best performance is achieved with:
- Direct containerd access (daemon = "containerd")
- Docker 23.0+ with containerd storage enabled and accessible containerd socket
""",
            default = "auto",
            values = ["auto", "docker", "containerd"],
        ),
        "tag": attr.string(
            doc = "Tag to apply when loading the image. Subject to [template expansion](/docs/templating.md).",
        ),
        "strategy": attr.string(
            doc = """Strategy for handling image layers during load.

Available strategies:
- **`auto`** (default): Uses the global default load strategy
- **`eager`**: Downloads all layers during the build phase. Ensures all layers are
  available locally before running the load command.
- **`lazy`**: Downloads layers only when needed during the load operation. More
  efficient for large images where some layers might already exist in the daemon.
""",
            default = "auto",
            values = ["auto", "eager", "lazy"],
        ),
        "build_settings": attr.string_keyed_label_dict(
            doc = "Build settings to use for [template expansion](/docs/templating.md). Keys are setting names, values are labels to string_flag targets.",
            providers = [BuildSettingInfo],
        ),
        "stamp": attr.string(
            doc = "Whether to use stamping for [template expansion](/docs/templating.md). If 'enabled', uses volatile-status.txt and version.txt if present. 'auto' uses the global default setting.",
            default = "auto",
            values = ["auto", "enabled", "disabled"],
        ),
        "_load_settings": attr.label(
            default = Label("//img/private/settings:load"),
            providers = [LoadSettingsInfo],
        ),
        "_stamp_settings": attr.label(
            default = Label("//img/private/settings:stamp"),
            providers = [StampSettingInfo],
        ),
        "_tool": attr.label(
            cfg = host_platform_transition,
            default = Label("//img:resolved_toolchain"),
        ),
    },
    executable = True,
    cfg = reset_platform_transition,
    toolchains = TOOLCHAINS,
)
