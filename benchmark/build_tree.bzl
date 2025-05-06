def _build_tree_impl(ctx):
    dir = ctx.actions.declare_directory(ctx.attr.name + "_dir")
    ctx.actions.run_shell(
        outputs = [dir],
        arguments = ctx.attr.names,
        command = "mkdir -p {path} {dirs}; cd {path} && truncate --size={size} $@".format(
            path = dir.path,
            dirs = dir.path + "/" + (" " + dir.path + "/").join(ctx.attr.dirs),
            size = ctx.attr.size,
        ),
    )
    return [DefaultInfo(files = depset([dir]))]

build_tree = rule(
    implementation = _build_tree_impl,
    attrs = {
        "size": attr.int(),
        "dirs": attr.string_list(),
        "names": attr.string_list(),
    },
)
