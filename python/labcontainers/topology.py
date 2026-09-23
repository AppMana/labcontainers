"""Serialization of upstream-schema-generated Containerlab objects."""

from . import containerlab_generated as containerlab
from . import labcontainers_pb2 as api


def source(config: containerlab.Config) -> api.TopologySource:
    """Encode explicitly supplied native fields, without injecting schema defaults.

    JSON is accepted by Containerlab's YAML parser. No YAML authoring or YAML
    dependency is needed in a test. Policy enforcement remains in the daemon.
    """
    if not isinstance(config, containerlab.Config):
        raise TypeError("expected a generated containerlab.Config object")
    return api.TopologySource(
        yaml=config.model_dump_json(by_alias=True, exclude_unset=True).encode("utf-8")
    )
