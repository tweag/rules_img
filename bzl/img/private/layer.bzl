"""Layer rule for building layers in a container image."""

load("//bzl/img:providers.bzl", "LayerInfo")
load(":layer_helper.bzl", "collect_content_manifests", "collect_required_layers")

def _file_type(f):
    type = "f"  # regular file
    if f.is_directory:
        type = "d"
    return type

def _files_arg(f):
    type = _file_type(f)
    return "{}{}".format(type, f.path)

def _to_short_path_pair(f):
    repo = f.owner.repo_name
    if repo == "":
        repo = "_main"
    type = _file_type(f)
    return "{}/{}\0{}{}".format(repo, f.short_path, type, f.path)

def _root_symlinks_arg(x):
    type = _file_type(x.target_file)
    return "{}\0{}{}".format(x.path, type, x.target_file.path)

def _symlinks_arg(x):
    type = _file_type(x.target_file)
    return "{}\0{}{}_main/{}".format(x.path, type, x.target_file.path)

def _symlink_tuple_to_arg(pair):
    source = pair[0]
    dest = pair[1]
    if source.startswith("/"):
        source = source[1:]
    return "{}\0{}".format(source, dest)

def _layer_impl(ctx):
    out = ctx.actions.declare_file(ctx.attr.name + ".tgz")
    metadata_out = ctx.actions.declare_file(ctx.attr.name + "_metadata.json")
    content_manifest_out = ctx.actions.declare_file(ctx.attr.name + ".content_manifest")
    args = ["layer", "--name", str(ctx.label), "--metadata", metadata_out.path, "--content-manifest", content_manifest_out.path]
    files_args = ctx.actions.args()
    files_args.set_param_file_format("multiline")
    files_args.use_param_file("--add-from-file=%s", use_always = True)

    inputs = []
    transitive_content_manifests = []
    required_layers = None

    for (pathInImage, files) in ctx.attr.srcs.items():
        default_info = files[DefaultInfo]
        files_to_run = default_info.files_to_run
        executable = None
        runfiles = None
        inputs.append(default_info.files)
        if files_to_run != None and files_to_run.executable != None and not files_to_run.executable.is_source:
            # This is an executable.
            # Add the executable with the runfiles tree, but ignore any other files.
            executable = files_to_run.executable
            runfiles = default_info.default_runfiles
            args.append("--executable={}={}".format(pathInImage, executable.path))
            executable_runfiles_args = ctx.actions.args()
            executable_runfiles_args.set_param_file_format("multiline")
            executable_runfiles_args.use_param_file("--runfiles={}=%s".format(executable.path), use_always = True)
            executable_runfiles_args.add_all(runfiles.files, map_each = _to_short_path_pair, expand_directories = False)
            executable_runfiles_args.add_all(runfiles.symlinks, map_each = _symlinks_arg)
            executable_runfiles_args.add_all(runfiles.root_symlinks, map_each = _root_symlinks_arg)
            args.append(executable_runfiles_args)
            inputs.append(runfiles.files)
            inputs.append(runfiles.symlinks)
            inputs.append(runfiles.root_symlinks)
            continue

        # This isn't an executable.
        # Let's add all files instead.
        if default_info.files == None:
            fail("Expected {} ({}) to contain an executable or files, got None".format(pathInImage, files))
        files_args.add_all(default_info.files, map_each = _files_arg, format_each = "{}\0%s".format(pathInImage), expand_directories = False)

    if len(ctx.attr.symlinks) > 0:
        symlink_args = ctx.actions.args()
        symlink_args.set_param_file_format("multiline")
        symlink_args.use_param_file("--symlinks-from-file=%s", use_always = True)
        symlink_args.add_all(ctx.attr.symlinks.items(), map_each = _symlink_tuple_to_arg)
        args.append(symlink_args)
    if hasattr(ctx.attr, "deduplicate") and ctx.attr.deduplicate != None:
        required_layers = collect_required_layers(ctx)
        collections = ctx.actions.args()
        collections.set_param_file_format("multiline")
        collections.use_param_file("--deduplicate-collection=%s", use_always = True)
        content_manifests = collect_content_manifests(ctx)
        collections.add_all(content_manifests)
        inputs.append(content_manifests)
        transitive_content_manifests.append(content_manifests)
        args.append(collections)
    args.append(files_args)
    args.append(out.path)

    ctx.actions.run(
        outputs = [out, metadata_out, content_manifest_out],
        inputs = depset(transitive = inputs),
        executable = ctx.executable._tool,
        arguments = args,
        mnemonic = "LayerTar",
    )
    return [
        DefaultInfo(files = depset([out])),
        OutputGroupInfo(
            layer = depset([out]),
            metadata = depset([metadata_out]),
            content_manifest = depset([content_manifest_out]),
        ),
        LayerInfo(
            blob = out,
            metadata = metadata_out,
            content_manifests = depset([content_manifest_out], transitive = transitive_content_manifests),
            required_layers = required_layers,
            media_type = "application/vnd.oci.image.layer.v1.tar+gzip",
        ),
    ]

layer = rule(
    implementation = _layer_impl,
    attrs = {
        "srcs": attr.string_keyed_label_dict(
            doc = "Files (including regular files, executables, and TreeArtifacts) that should be added to the layer.",
            allow_files = True,
        ),
        "symlinks": attr.string_dict(
            doc = "Symlinks that should be added to the layer.",
        ),
        "deduplicate": attr.label_list(
            doc = """Optional layers that are known to be below this layer.
Any files included in referenced layers will not be written again.
Users are free to choose: adding a layer here adds an ordering constraint (referenced layers have to be built first), but doing so can reduce image size.""",
            providers = [LayerInfo],
        ),
        "_tool": attr.label(
            executable = True,
            cfg = "exec",
            default = Label("//cmd/img"),
        ),
    },
    provides = [LayerInfo],
)
