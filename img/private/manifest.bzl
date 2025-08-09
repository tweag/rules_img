"""Image rule for assembling OCI images based on a set of layers."""

load("//img/private/common:build.bzl", "TOOLCHAIN", "TOOLCHAINS")
load("//img/private/common:transitions.bzl", "normalize_layer_transition")
load("//img/private/config:defs.bzl", "TargetPlatformInfo")
load("//img/private/providers:index_info.bzl", "ImageIndexInfo")
load("//img/private/providers:layer_info.bzl", "LayerInfo")
load("//img/private/providers:manifest_info.bzl", "ImageManifestInfo")
load("//img/private/providers:pull_info.bzl", "PullInfo")

_GOOS = [
    "android",
    "darwin",
    "dragonfly",
    "freebsd",
    "illumos",
    "ios",
    "js",
    "linux",
    "netbsd",
    "openbsd",
    "plan9",
    "solaris",
    "wasip1",
    "windows",
]

_GOARCH = [
    "amd64",
    "386",
    "arm",
    "arm64",
    "ppc64le",
    "ppc64",
    "mips64le",
    "mips64",
    "mipsle",
    "mips",
    "s390x",
    "wasm",
]

def _to_layer_arg(layer):
    """Convert a layer to a command line argument."""
    return layer.metadata.path

def _platform_matches(wanted_platform, manifest):
    """Check if the wanted platform matches the manifest platform."""
    if wanted_platform["os"] != manifest.os:
        return False
    if wanted_platform["architecture"] != manifest.architecture:
        return False
    for key in wanted_platform["platform"].keys():
        if key not in manifest:
            return False
        if wanted_platform[key] != manifest[key]:
            return False
    return True

def select_base(ctx):
    """Select the base image to use for this image.

    Args:
        ctx: Rule context containing base image information.

    Returns:
        ImageManifestInfo for the selected base image, or None if no base.
    """
    if ctx.attr.base == None:
        return None
    if ImageManifestInfo in ctx.attr.base:
        return ctx.attr.base[ImageManifestInfo]
    if ImageIndexInfo not in ctx.attr.base:
        fail("base image must be an ImageManifestInfo or ImageIndexInfo")

    os_wanted = ctx.attr.os if ctx.attr.os != "" else "linux"
    arch_wanted = ctx.attr.architecture if ctx.attr.architecture != "" else ctx.attr._os_cpu[TargetPlatformInfo].cpu
    constraints_wanted = dict(
        os = os_wanted,
        architecture = arch_wanted,
        platform = ctx.attr.platform,
    )

    for manifest in ctx.attr.base[ImageIndexInfo].manifests:
        if _platform_matches(constraints_wanted, manifest):
            return manifest
    fail("no matching base image found for architecture {} and os {}".format(constraints_wanted["architecture"], constraints_wanted["os"]))

def _build_oci_layout(ctx, manifest_out, config_out, layers):
    """Build the OCI layout for the image.

    Args:
        ctx: Rule context.
        manifest_out: The manifest file.
        config_out: The config file.
        layers: List of LayerInfo providers.

    Returns:
        The OCI layout directory (tree artifact).
    """
    oci_layout_dir = ctx.actions.declare_directory(ctx.label.name + "_oci_layout")

    args = ctx.actions.args()
    args.add("oci-layout")
    args.add("--manifest", manifest_out.path)
    args.add("--config", config_out.path)
    args.add("--output", oci_layout_dir.path)

    inputs = [manifest_out, config_out]

    # Add layers with metadata=blob mapping
    for layer in layers:
        if layer.blob != None:
            args.add("--layer", "{}={}".format(layer.metadata.path, layer.blob.path))
            inputs.append(layer.metadata)
            inputs.append(layer.blob)

    img_toolchain_info = ctx.toolchains[TOOLCHAIN].imgtoolchaininfo
    ctx.actions.run(
        inputs = inputs,
        outputs = [oci_layout_dir],
        executable = img_toolchain_info.tool_exe,
        arguments = [args],
        mnemonic = "OCILayout",
    )

    return oci_layout_dir

def _image_manifest_impl(ctx):
    inputs = []
    providers = []
    args = ctx.actions.args()
    args.add("manifest")
    base = select_base(ctx)
    os = None
    arch = None
    history = []
    layers = []
    if base != None:
        if ctx.attr.os != "" and ctx.attr.os != base.os:
            fail("base image OS {} does not match requested OS {}".format(base.os, ctx.attr.os))
        if ctx.attr.architecture != "" and ctx.attr.architecture != base.architecture:
            fail("base image architecture {} does not match requested architecture {}".format(base.architecture, ctx.attr.architecture))
        os = base.os
        arch = base.architecture
        history = base.structured_config.get("history", [])
        layers.extend(base.layers)
        inputs.append(base.manifest)
        inputs.append(base.config)
        args.add("--base-manifest", base.manifest.path)
        args.add("--base-config", base.config.path)
    else:
        os = ctx.attr.os if ctx.attr.os != "" else "linux"
        arch = ctx.attr.architecture if ctx.attr.architecture != "" else ctx.attr._os_cpu[TargetPlatformInfo].cpu
    if ctx.attr.base != None and PullInfo in ctx.attr.base:
        providers.append(ctx.attr.base[PullInfo])
    for layer in ctx.attr.layers:
        layers.append(layer[LayerInfo])

    args.add("--os", os)
    args.add("--architecture", arch)

    # todo: encode platform metadata
    for layer in layers:
        inputs.append(layer.metadata)
    args.add_all(layers, format_each = "--layer-from-metadata=%s", map_each = _to_layer_arg, expand_directories = False)
    if ctx.attr.config_fragment != None:
        inputs.append(ctx.file.config_fragment)
        args.add("--config-fragment", ctx.file.config_fragment.path)

    # Add image config attributes
    if ctx.attr.user:
        args.add("--user", ctx.attr.user)
    for key, value in ctx.attr.env.items():
        args.add("--env", "%s=%s" % (key, value))
    for entry in ctx.attr.entrypoint:
        args.add("--entrypoint", entry)
    for entry in ctx.attr.cmd:
        args.add("--cmd", entry)
    if ctx.attr.working_dir:
        args.add("--working-dir", ctx.attr.working_dir)
    for key, value in ctx.attr.labels.items():
        args.add("--label", "%s=%s" % (key, value))
    if ctx.attr.stop_signal:
        args.add("--stop-signal", ctx.attr.stop_signal)
    for key, value in ctx.attr.annotations.items():
        args.add("--annotation", "%s=%s" % (key, value))

    structured_config = dict(
        architecture = arch,
        os = os,
        history = history,
    )

    manifest_out = ctx.actions.declare_file(ctx.label.name + "_manifest.json")
    config_out = ctx.actions.declare_file(ctx.label.name + "_config.json")
    descriptor_out = ctx.actions.declare_file(ctx.label.name + "_descriptor.json")
    args.add("--manifest", manifest_out.path)
    args.add("--config", config_out.path)
    args.add("--descriptor", descriptor_out.path)

    img_toolchain_info = ctx.toolchains[TOOLCHAIN].imgtoolchaininfo
    ctx.actions.run(
        inputs = inputs,
        outputs = [manifest_out, config_out, descriptor_out],
        executable = img_toolchain_info.tool_exe,
        arguments = [args],
        mnemonic = "ImageManifest",
    )

    providers.extend([
        DefaultInfo(
            files = depset([manifest_out, config_out]),
        ),
        OutputGroupInfo(
            descriptor = depset([descriptor_out]),
            oci_layout = depset([_build_oci_layout(ctx, manifest_out, config_out, layers)]),
        ),
        ImageManifestInfo(
            base_image = base,
            descriptor = descriptor_out,
            manifest = manifest_out,
            config = config_out,
            structured_config = structured_config,
            architecture = arch,
            os = os,
            platform = ctx.attr.platform,
            layers = layers,
            missing_blobs = base.missing_blobs if base != None else [],
        ),
    ])
    return providers

image_manifest = rule(
    implementation = _image_manifest_impl,
    doc = """Builds a single-platform OCI container image from a set of layers.

This rule assembles container images by combining:
- Optional base image layers (from another image_manifest or image_index)
- Additional layers created by image_layer rules
- Image configuration (entrypoint, environment, labels, etc.)

The rule produces:
- OCI manifest and config JSON files
- An optional OCI layout directory (via output groups)
- ImageManifestInfo provider for use by image_index or image_push

Example:
```python
image_manifest(
    name = "my_app",
    base = "@distroless_cc",
    layers = [
        ":app_layer",
        ":config_layer",
    ],
    entrypoint = ["/usr/bin/app"],
    env = {
        "APP_ENV": "production",
    },
)
```

Output groups:
- `descriptor`: OCI descriptor JSON file
- `oci_layout`: Complete OCI layout directory with blobs
""",
    attrs = {
        "base": attr.label(
            doc = "Base image to inherit layers from. Should provide ImageManifestInfo or ImageIndexInfo.",
        ),
        "layers": attr.label_list(
            providers = [LayerInfo],
            doc = "Layers to include in the image.",
            cfg = normalize_layer_transition,
        ),
        "os": attr.string(
            values = _GOOS,
            doc = "The operating system this image runs on.",
        ),
        "architecture": attr.string(
            values = _GOARCH,
            doc = "The CPU architecture this image runs on.",
        ),
        "platform": attr.string_dict(
            default = {},
            doc = "Dict containing additional runtime requirements of the image.",
        ),
        "user": attr.string(
            doc = """The username or UID which is a platform-specific structure that allows specific control over which user the process run as.
This acts as a default value to use when the value is not specified when creating a container.""",
        ),
        "env": attr.string_dict(
            doc = "Default environment variables to set when starting a container based on this image.",
            default = {},
        ),
        "entrypoint": attr.string_list(
            doc = "A list of arguments to use as the command to execute when the container starts. These values act as defaults and may be replaced by an entrypoint specified when creating a container.",
            default = [],
        ),
        "cmd": attr.string_list(
            doc = "Default arguments to the entrypoint of the container. These values act as defaults and may be replaced by any specified when creating a container. If an Entrypoint value is not specified, then the first entry of the Cmd array SHOULD be interpreted as the executable to run.",
            default = [],
        ),
        "working_dir": attr.string(
            doc = "Sets the current working directory of the entrypoint process in the container. This value acts as a default and may be replaced by a working directory specified when creating a container.",
        ),
        "labels": attr.string_dict(
            doc = "This field contains arbitrary metadata for the container.",
            default = {},
        ),
        "annotations": attr.string_dict(
            doc = "This field contains arbitrary metadata for the manifest.",
            default = {},
        ),
        "stop_signal": attr.string(
            doc = "This field contains the system call signal that will be sent to the container to exit. The signal can be a signal name in the format SIGNAME, for instance SIGKILL or SIGRTMIN+3.",
        ),
        "config_fragment": attr.label(
            doc = "Optional JSON file containing a partial image config, which will be used as a base for the final image config.",
            allow_single_file = True,
        ),
        "_os_cpu": attr.label(
            default = Label("//img/private/config:target_os_cpu"),
            providers = [TargetPlatformInfo],
        ),
    },
    provides = [ImageManifestInfo],
    toolchains = TOOLCHAINS,
)
