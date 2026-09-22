"""Opt-in guest-observed qualification of declared-NIC VM wrappers."""
import json
import os
from pathlib import Path
import time
import unittest

from labcontainers import Client, api, containerlab as clab, source


class VMNICChecks:
    image_variable = "LABCONTAINERS_NICS_LIVE_IMAGE"

    def observe(self, node):
        result = node.exec("ip", "-json", "link", "show", timeout_seconds=10)
        result.check_returncode()
        devices = json.loads(result.stdout)
        ethernet = [device for device in devices if device.get("link_type") == "ether"]
        routes = []
        for family in ("-4", "-6"):
            result = node.exec("ip", family, "-json", "route", "show", "default")
            result.check_returncode()
            routes.extend(json.loads(result.stdout))
        return ethernet, routes

    def test_zero_and_two_nics_have_no_implicit_network(self):
        image = os.environ[self.image_variable]
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
                lab = client.start(api.LabSpec(topology=source(config), nodes={"vm": api.NodeExtension(control="qga")}), ttl_seconds=600)
                node = lab.node("vm")
                deadline = time.monotonic() + 420
                last_error = None
                while time.monotonic() < deadline:
                    try:
                        ethernet, routes = self.observe(node)
                        break
                    except Exception as error:
                        last_error = error
                        time.sleep(2)
                else:
                    self.fail(f"QGA never answered for {count} NICs: {last_error}")
                self.assertEqual(len(ethernet), count, ethernet)
                self.assertEqual(routes, [])
                print(f"guest-observed {count} NICs, no IPv4/IPv6 default route; evidence: {lab.value.artifact_directory}", flush=True)


@unittest.skipUnless(os.environ.get("LABCONTAINERS_NICS_LIVE_IMAGE"), "requires prepared declared-NIC VM image and KVM")
class VMNICLiveTests(VMNICChecks, unittest.TestCase):
    pass


@unittest.skipUnless(os.environ.get("LABCONTAINERS_WINDOWS_NICS_LIVE_IMAGE"), "requires prepared Windows declared-NIC image and KVM")
class WindowsNICLiveTests(VMNICChecks, unittest.TestCase):
    image_variable = "LABCONTAINERS_WINDOWS_NICS_LIVE_IMAGE"

    def observe(self, node):
        script = r"""
$ErrorActionPreference = 'Stop'
$adapters = @(Get-NetAdapter -IncludeHidden | Where-Object { $_.HardwareInterface } | Select-Object Name, InterfaceDescription, Status)
$routes = @(Get-NetRoute | Where-Object { $_.DestinationPrefix -in @('0.0.0.0/0', '::/0') } | Select-Object DestinationPrefix, NextHop, InterfaceAlias)
$build = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
@{adapters=$adapters; routes=$routes; version=('10.0.' + $build.CurrentBuildNumber + '.' + $build.UBR)} | ConvertTo-Json -Depth 5 -Compress
"""
        result = node.exec("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script, timeout_seconds=30)
        result.check_returncode()
        observed = json.loads(result.stdout.decode("utf-8-sig"))
        print(f"Windows guest version: {observed['version']}", flush=True)
        return observed["adapters"], observed["routes"]
