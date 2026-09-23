"""Cloud-init's generated network types and their file serialization boundary."""

import os
from pathlib import Path
from . import cloudinit_generated as models


def write_network_config(filename: str | Path, config: models.NetworkConfigVersion2) -> None:
    """Write the caller's generated object without inserting network defaults."""
    if not isinstance(config, models.NetworkConfigVersion2):
        raise TypeError("expected a generated cloud-init NetworkConfigVersion2")
    data = config.model_dump_json(by_alias=True, exclude_unset=True).encode("utf-8")
    models.NetworkConfigVersion2.model_validate_json(data)
    fd = os.open(filename, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "wb") as stream:
        stream.write(data)
