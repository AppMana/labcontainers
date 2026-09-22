"""Opt-in guest-observed qualification of the declared-NIC Ubuntu wrapper."""
import json
import os
from pathlib import Path
import time
import unittest

from labcontainers import Client, api, containerlab as clab, source


@unittest.skipUnless(os.environ.get("LABCONTAINERS_NICS_LIVE_IMAGE"), "requires prepared declared-NIC VM image and KVM")
class VMNICLiveTests(unittest.TestCase):
    def test_zero_and_two_nics_have_no_implicit_network(self):
        image = os.environ["LABCONTAINERS_NICS_LIVE_IMAGE"]
        labd = Path(__file__).resolve().parents[2] / "bin/labd"
        for count in (0, 2):
            with self.subTest(nics=count), Client(labd=str(labd)) as client:
                nodes = {"vm": clab.NodeConfig(kind="generic_vm", image=image, image_pull_policy="Never")}
                links = []
                if count:
                    nodes["peer"] = clab.NodeConfig(kind="linux", image="alpine:3.20", image_pull_policy="Never")
                    links = [clab.LinkConfigShort(endpoints=[f"vm:eth{i}", f"peer:eth{i}"]) for i in range(1, count + 1)]
                # Native schema permits omitted links, not an empty links list.
                topology = clab.Topology(nodes=nodes, **({"links": links} if links else {}))
                config = clab.Config(name="native-nics", topology=topology)
                lab = client.start(api.LabSpec(topology=source(config), nodes={"vm": api.NodeExtension(control="qga")}), ttl_seconds=300)
                node = lab.node("vm")
                deadline = time.monotonic() + 180
                last_error = None
                while time.monotonic() < deadline:
                    try:
                        result = node.exec("ip", "-json", "link", "show", timeout_seconds=10)
                        result.check_returncode()
                        devices = json.loads(result.stdout)
                        break
                    except Exception as error:
                        last_error = error
                        time.sleep(2)
                else:
                    self.fail(f"QGA never answered for {count} NICs: {last_error}")
                ethernet = [device for device in devices if device.get("link_type") == "ether"]
                self.assertEqual(len(ethernet), count, devices)
                for family in ("-4", "-6"):
                    routes = node.exec("ip", family, "-json", "route", "show", "default")
                    routes.check_returncode()
                    self.assertEqual(json.loads(routes.stdout), [])
                print(f"guest-observed {count} NICs, no IPv4/IPv6 default route; evidence: {lab.value.artifact_directory}", flush=True)
