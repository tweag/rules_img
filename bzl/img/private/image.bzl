"""Image rule for assembling OCI images based on a set of layers."""

load("//bzl/img:providers.bzl", "ImageIndexInfo", "ImageManifestInfo", "LayerInfo", "PullInfo")
load("//bzl/img/private/config:defs.bzl", "TargetPlatformInfo")

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
    """Select the base image to use for this image."""
    if ctx.attr.base == None:
        return None
    if ImageManifestInfo in ctx.attr.base:
        return ctx.attr.base[ImageManifestInfo]
    if ImageIndexInfo not in ctx.attr.base:
        fail("base image must be an ImageManifestInfo or ImageIndexInfo")

    os_wanted = ctx.attr.os if ctx.attr.os != None else "linux"
    arch_wanted = ctx.attr.architecture if ctx.attr.architecture != None else ctx.attr._os_cpu[TargetPlatformInfo].cpu
    constraints_wanted = dict(
        os = os_wanted,
        architecture = arch_wanted,
        platform = ctx.attr.platform,
    )

    for manifest in ctx.attr.base[ImageIndexInfo].manifests:
        if _platform_matches(constraints_wanted, manifest):
            return manifest
    fail("no matching base image found for architecture {} and os {}".format(ctx.attr.architecture, ctx.attr.os))

def _layer_presence_validation(ctx, layers):
    layer_args = ctx.actions.args()
    required_layer_param_file = ctx.actions.args()
    required_layer_param_file.set_param_file_format("multiline")
    required_layer_param_file.use_param_file("@%s", use_always = True)
    transitive_inputs = []
    for (i, layer) in enumerate(layers):
        layer_args.add("--layer-metadata", layer.metadata.path, format = "{}=%s".format(i))
        if layer.required_layers == None:
            continue
        required_layer_param_file.add_all(layer.required_layers, format_each = "{}\0%s".format(i))
        transitive_inputs.append(layer.required_layers)
    presence_validation_output = ctx.actions.declare_file(ctx.attr.name + ".layer_presence")
    ctx.actions.run(
        outputs = [presence_validation_output],
        inputs = depset([layer.metadata for layer in layers], transitive = transitive_inputs),
        executable = ctx.executable._tool,
        arguments = ["validate", "layer-presence", "--file", presence_validation_output.path, layer_args, required_layer_param_file],
        mnemonic = "ImageIntegrity",
        progress_message = "Checking presence of layers used for deduplication in %{label}",
    )
    return presence_validation_output

def _image_impl(ctx):
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
        if ctx.attr.os == "":
            fail("no base image provided and no OS specified")
        if ctx.attr.architecture == "":
            fail("no base image provided and no architecture specified")
        os = ctx.attr.os
        arch = ctx.attr.architecture
    if PullInfo in ctx.attr.base:
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

    ctx.actions.run(
        inputs = inputs,
        outputs = [manifest_out, config_out, descriptor_out],
        executable = ctx.executable._tool,
        arguments = [args],
        mnemonic = "ImageManifest",
    )
    presence_validation_output = _layer_presence_validation(ctx, layers)

    providers.extend([
        DefaultInfo(
            files = depset([manifest_out, config_out]),
        ),
        OutputGroupInfo(
            descriptor = depset([descriptor_out]),
            _validation = depset([presence_validation_output]),
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
            missing_blobs = base.missing_blobs,
        ),
    ])
    return providers

image = rule(
    implementation = _image_impl,
    attrs = {
        "base": attr.label(
            doc = "Base image to inherit layers from. Should provide ImageManifestInfo or ImageIndexInfo.",
        ),
        "layers": attr.label_list(
            providers = [LayerInfo],
            doc = "Layers to include in the image.",
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
        "config_fragment": attr.label(
            doc = "Optional JSON file containing a partial image config, which will be used as a base for the final image config.",
            allow_single_file = True,
        ),
        "_os_cpu": attr.label(
            default = Label("//bzl/img/private/config:target_os_cpu"),
            providers = [TargetPlatformInfo],
        ),
        "_tool": attr.label(
            executable = True,
            cfg = "exec",
            default = Label("//cmd/img"),
        ),
    },
    provides = [ImageManifestInfo],
)
