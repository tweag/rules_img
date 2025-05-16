load("@rules_img//bzl/img/private:image.bzl", "image")
load("@with_cfg.bzl", "with_cfg")

def platform_image(platform):
    _builder = with_cfg(image)
    _builder.set("platforms", [Label(platform)])
    platform_image, _platform_image_internal = _builder.build()
    return platform_image

def multi_platform_image(platforms, **kwargs):
    for platform in platforms:
        platform_image_rule = platform_image(platform)
        platform_image_rule(**kwargs)
