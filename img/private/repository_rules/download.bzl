def download_blob(rctx, *, digest, **kwargs):
    sha256 = digest.removeprefix("sha256:")
    output = "blobs/sha256/" + sha256
    registries = [r for r in rctx.attr.registries]
    if rctx.attr.registry:
        registries.append(rctx.attr.registry)
    if len(registries) == 0:
        fail("need at least one registry to pull from")
    rctx.download(
        url = [
            "{protocol}://{registry}/v2/{repository}/blobs/{digest}".format(
                protocol = "https",
                registry = registry,
                repository = rctx.attr.repository,
                digest = digest,
            )
            for registry in registries
        ],
        sha256 = sha256,
        output = output,
        **kwargs
    )
    return struct(
        digest = digest,
        path = output,
        data = rctx.read(output),
    )

def download_manifest(rctx, *, reference, **kwargs):
    registries = [r for r in rctx.attr.registries]
    have_valid_digest = False
    if rctx.attr.registry:
        registries.append(rctx.attr.registry)
    if len(registries) == 0:
        fail("need at least one registry to pull from")
    if reference.startswith("sha256:"):
        have_valid_digest = True
        sha256 = reference.removeprefix("sha256:")
        kwargs["sha256"] = sha256
        kwargs["output"] = "blobs/sha256/" + sha256
    else:
        kwargs["output"] = "manifest.json"
    manifest_result = rctx.download(
        url = [
            "{protocol}://{registry}/v2/{repository}/manifests/{reference}".format(
                protocol = "https",
                registry = registry,
                repository = rctx.attr.repository,
                reference = reference,
            )
            for registry in registries
        ],
        **kwargs
    )
    if not have_valid_digest:
        fail("""Missing valid image digest. Observed the following digest when pulling manifest for {}:
    sha256:{}""".format(
            rctx.attr.repository + ":" + rctx.attr.tag,
            manifest_result.sha256,
        ))
    return struct(
        digest = reference,
        path = kwargs["output"],
        data = rctx.read(kwargs["output"]),
    )
