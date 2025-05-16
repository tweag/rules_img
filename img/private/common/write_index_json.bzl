def _annotation_arg(tup):
    return "{}={}".format(tup[0], tup[1])

def write_index_json(ctx, *, output, manifests, annotations):
    manifest_descriptors = [manifest.descriptor for manifest in manifests]
    args = ctx.actions.args()
    args.add("index")
    args.add_all(manifest_descriptors, format_each = "--manifest-descriptor=%s")
    args.add_all(ctx.attr.annotations.items(), map_each = _annotation_arg, format_each = "--annotation=%s")
    args.add(output.path)
    ctx.actions.run(
        outputs = [output],
        inputs = manifest_descriptors,
        executable = ctx.executable._tool,
        arguments = [args],
        mnemonic = "ImageIndex",
    )
