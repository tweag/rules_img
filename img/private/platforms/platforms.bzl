"""Set of supported execution platforms."""

LINUX = "linux"
DARWIN = "darwin"
WINDOWS = "windows"

AMD64 = "amd64"
ARM64 = "arm64"
I386 = "386"

def _bazel_os(name):
    if name == DARWIN:
        return "macos"
    return name

def _bazel_cpu(name):
    if name == AMD64:
        return "x86_64"
    if name == I386:
        return "i386"
    return name

def _platform_os(name):
    if name == "macos":
        return DARWIN
    return name

def _platform_cpu(name):
    if name == "x86_64":
        return AMD64
    if name == "i386":
        return I386
    return name

def _constraints(tup):
    return [
        "@platforms//os:" + _bazel_os(tup[0]),
        "@platforms//cpu:" + _bazel_cpu(tup[1]),
    ]

def _platform_name(tup):
    return tup[0] + "_" + tup[1]

_tuples = [
    # linux
    (LINUX, AMD64),
    (LINUX, ARM64),
    (LINUX, I386),

    # darwin
    (DARWIN, AMD64),
    (DARWIN, ARM64),

    # windows
    (WINDOWS, AMD64),
    (WINDOWS, ARM64),
]

PLATFORMS = {
    _platform_name(tup): struct(
        platform_info = str(Label(":" + _platform_name(tup))),
        constraints = _constraints(tup),
    )
    for tup in _tuples
}

def platform_for_goos_and_goarch(mangled_name):
    [goos, goarch] = mangled_name.split("_")
    os = _platform_os(goos)
    cpu = _platform_cpu(goarch)
    return PLATFORMS[_platform_name((os, cpu))]
