import unittest
import json
import tempfile
from pathlib import Path
from pydantic import ValidationError
from labcontainers import cloudinit_generated as ci
from labcontainers.cloudinit import write_network_config


class CloudInitSchemaTests(unittest.TestCase):
    def test_writer_has_no_caller_authored_yaml(self):
        with tempfile.TemporaryDirectory() as root:
            filename = Path(root) / "network.json"
            write_network_config(filename, ci.NetworkConfigVersion2(version=2))
            self.assertEqual(json.loads(filename.read_text()), {"version": 2})
            self.assertEqual(filename.stat().st_mode & 0o777, 0o600)

    def test_network_has_only_explicit_choices(self):
        config = ci.NetworkConfigVersion2(version=2, ethernets={
            "data": ci.MappingPhysical(match=ci.Match(name="en*"), addresses=["192.0.2.1/24"], optional=True),
        })
        self.assertEqual(config.model_dump(mode="json", by_alias=True, exclude_unset=True), {
            "version": 2, "ethernets": {"data": {"match": {"name": "en*"}, "addresses": ["192.0.2.1/24"], "optional": True}},
        })

    def test_open_network_fields_survive(self):
        config = ci.MappingPhysical.model_validate({"accept-ra": False, "dhcp4": False, "dhcp6": "no"})
        self.assertEqual(config.model_dump(mode="json", by_alias=True, exclude_unset=True), {
            "accept-ra": False, "dhcp4": False, "dhcp6": "no",
        })

    def test_closed_root_rejects_unknown_fields(self):
        with self.assertRaises(ValidationError):
            ci.NetworkConfigVersion2(version=2, ethernet={})
