"""Helper functions for creating file metadata for image layers."""

def file_metadata(
        *,
        mode = None,
        uid = None,
        gid = None,
        uname = None,
        gname = None,
        mtime = None,
        pax_records = None):
    """Creates a JSON-encoded file metadata string for use with image_layer rules.

    This function generates JSON metadata that can be used to customize file attributes
    in container image layers, such as permissions, ownership, and timestamps.

    Args:
        mode: File permission mode (e.g., "0755", "0644"). String format.
        uid: User ID of the file owner. Integer.
        gid: Group ID of the file owner. Integer.
        uname: User name of the file owner. String.
        gname: Group name of the file owner. String.
        mtime: Modification time in RFC3339 format (e.g., "2023-01-01T00:00:00Z"). String.
        pax_records: Dict of extended attributes to set via PAX records.

    Returns:
        JSON-encoded string containing the file metadata.
    """
    metadata = {}

    if mode != None:
        metadata["mode"] = mode
    if uid != None:
        metadata["uid"] = uid
    if gid != None:
        metadata["gid"] = gid
    if uname != None:
        metadata["uname"] = uname
    if gname != None:
        metadata["gname"] = gname
    if mtime != None:
        metadata["mtime"] = mtime
    if pax_records != None:
        metadata["pax_records"] = pax_records

    return json.encode(metadata)
