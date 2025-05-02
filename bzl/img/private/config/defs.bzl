TargetPlatformInfo = provider(
    doc = "Information on the target platform.",
    fields = {
        "os": "The OS (as GOOS)",
        "cpu": "The cpu / arch (as GOARCH)",
    },
)

def _os_cpu_impl(ctx):
    return [TargetPlatformInfo(
        os = ctx.attr.os,
        cpu = ctx.attr.cpu,
    )]

os_cpu = rule(
    implementation = _os_cpu_impl,
    attrs = {
        "os": attr.string(),
        "cpu": attr.string(),
    },
)
