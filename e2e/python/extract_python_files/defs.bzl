"""Rules for preparing Python executables for container images.

This module provides rules to extract Python executables and their dependencies
for inclusion in container images, without bundling the Python interpreter or
standard library (which should come from the base image).
"""

load("@rules_python//python:py_executable_info.bzl", "PyExecutableInfo")

def _py_executable_for_image_impl(ctx):
    executable_info = ctx.attr.binary[PyExecutableInfo]
    main = executable_info.main
    runfiles = executable_info.runfiles_without_exe
    out = ctx.actions.declare_file(ctx.attr.name)
    ctx.actions.expand_template(
        output = out,
        template = main,
        is_executable = True,
    )

    return [
        DefaultInfo(
            files = depset([out]),
            executable = out,
            runfiles = runfiles,
        ),
    ]

py_executable_for_image = rule(
    implementation = _py_executable_for_image_impl,
    attrs = {
        "binary": attr.label(
            providers = [PyExecutableInfo],
            mandatory = True,
            doc = "The Python binary.",
        ),
    },
    doc = """Creates a Python executable for use in an image.
This includes the script and its dependencies, but skips the interpreter and standard library.""",
    executable = True,
)
