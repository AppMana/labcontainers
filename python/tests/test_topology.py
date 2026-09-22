import json
import unittest

from pydantic import ValidationError

from labcontainers import containerlab as clab, source


class TopologyTests(unittest.TestCase):
    def test_native_fields_and_open_variables_survive(self):
        config = clab.Config(
            name="typed",
            topology=clab.Topology(
                defaults=clab.NodeConfig(kind="linux", image="alpine:3.20"),
                nodes={
                    "client": clab.NodeConfig(network_mode="none", auto_remove=False),
                    "server": clab.NodeConfig(network_mode="none"),
                },
                links=[clab.LinkTypeVeth(
                    type="veth",
                    endpoints=[
                        clab.LinkEndpoint(node="client", interface="eth1", ipv4="192.0.2.1/24"),
                        clab.LinkEndpoint(node="server", interface="eth1", ipv4="192.0.2.2/24"),
                    ],
                    vars=clab.LinkVars(arbitrary={"zero": 0, "disabled": False}),
                )],
            ),
        )
        payload = json.loads(source(config).yaml)
        self.assertEqual(set(payload), {"name", "topology"})
        self.assertEqual(payload["topology"]["nodes"]["client"], {"network-mode": "none", "auto-remove": False})
        link = payload["topology"]["links"][0]
        self.assertEqual(link["vars"], {"arbitrary": {"zero": 0, "disabled": False}})
        self.assertNotIn("mtu", link)
        self.assertNotIn("mgmt", payload)
        self.assertEqual(link["endpoints"][0]["interface"], "eth1")

    def test_brief_link_does_not_emit_schema_defaults(self):
        config = clab.Config(name="minimal", topology=clab.Topology(
            nodes={"a": clab.NodeConfig(), "b": clab.NodeConfig()},
            links=[clab.LinkConfigShort(endpoints=["a:eth1", "b:eth1"])],
        ))
        self.assertEqual(json.loads(source(config).yaml)["topology"]["links"],
                         [{"endpoints": ["a:eth1", "b:eth1"]}])

    def test_closed_schema_rejects_typo(self):
        with self.assertRaises(ValidationError):
            clab.NodeConfig(network_mdoe="none")

    def test_source_requires_generated_config(self):
        with self.assertRaises(TypeError):
            source({"name": "not-a-generated-object"})


if __name__ == "__main__":
    unittest.main()
