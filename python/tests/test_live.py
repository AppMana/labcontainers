"""Opt-in, code-only test against the real daemon and Containerlab."""

import os
from pathlib import Path
import unittest

from labcontainers import Client, api, containerlab as clab, source


@unittest.skipUnless(os.environ.get("LABCONTAINERS_LIVE") == "1", "requires privileged Containerlab")
class LiveTests(unittest.TestCase):
    def test_declared_path_is_the_only_path(self):
        config = clab.Config(name="python", topology=clab.Topology(
            defaults=clab.NodeConfig(kind="linux", image="alpine:3.20"),
            nodes={
                "client": clab.NodeConfig(exec=["ip addr add 192.0.2.1/24 dev eth0"]),
                "server": clab.NodeConfig(exec=["ip addr add 192.0.2.2/24 dev eth0"]),
            },
            links=[clab.LinkConfigShort(endpoints=["client:eth0", "server:eth0"])],
        ))
        labd = Path(__file__).resolve().parents[2] / "bin" / "labd"
        with Client(labd=str(labd)) as client:
            lab = client.start(api.LabSpec(topology=source(config)), ttl_seconds=300)
            plan = lab.plan(source(config))
            self.assertTrue(plan["dry-run"])
            self.assertEqual(plan["recreated-nodes"], [])
            draft = config.model_copy(deep=True)
            draft.topology.nodes["adhoc"] = clab.NodeConfig()
            addition = lab.plan(source(draft))
            self.assertEqual(addition["added-nodes"], ["adhoc"])
            self.assertEqual(lab.plan()["added-nodes"], [])
            if os.environ.get("LABCONTAINERS_RECONCILE_LIVE"):
                self.assertEqual(addition["recreated-nodes"], [])
                self.assertEqual(addition["restarted-nodes"], [])
                lab.apply(source(draft), addition)
                lab.node("adhoc").exec("true").check_returncode()
                removal = lab.plan(source(config))
                self.assertEqual(removal["deleted-nodes"], ["adhoc"])
                self.assertEqual(removal["recreated-nodes"], [])
                lab.apply(source(config), removal)
            probe = lambda: lab.node("client").exec("ping", "-c", "1", "-W", "1", "192.0.2.2")
            probe().check_returncode()
            fault = lab.set_link("client", "eth0", False)
            self.assertNotEqual(probe().returncode, 0)
            fault.revert()
            probe().check_returncode()
