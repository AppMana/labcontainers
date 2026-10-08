import importlib.util
import json
import os
import pathlib
import subprocess
import sys
import time
import types
import unittest
from unittest.mock import Mock, patch


class WindowsLauncherHealthTests(unittest.TestCase):
    def launcher(self, base_vm=object):
        path = pathlib.Path(__file__).resolve().parents[2] / "images/windows-server-2022/launcher.py"
        spec = importlib.util.spec_from_file_location("windows_health_test_launcher", path)
        module = importlib.util.module_from_spec(spec)
        vrnetlab = types.SimpleNamespace(VM=base_vm, VR=object)
        interfaces = types.SimpleNamespace(declared_nics=Mock(), die_with_launcher=Mock(), die_with_parent=Mock(), isolate_control_listeners=Mock(), use_netns_interface_view=Mock(), wait_for_interfaces=Mock())
        with patch.dict(sys.modules, vrnetlab=vrnetlab, interfaces=interfaces):
            spec.loader.exec_module(module)
        return module

    def test_restart_never_uses_an_existing_overlay_as_base(self):
        class BaseVM:
            def __init__(self, *args, disk_image, **kwargs):
                self.disk_image = disk_image
                self.qemu_args = []

        module = self.launcher(BaseVM)
        with patch.object(module.os, "listdir", return_value=[
            "windows-overlay-overlay.qcow2", "windows-overlay.qcow2", "windows.qcow2",
        ]), patch.object(module.Path, "glob", return_value=[]):
            vm = module.WindowsVM(0, "tc")
        self.assertEqual(vm.disk_image, "/windows.qcow2")

    def test_multiple_base_disks_fail_instead_of_selecting_arbitrarily(self):
        module = self.launcher()
        with patch.object(module.os, "listdir", return_value=["one.qcow2", "two.qcow2"]):
            with self.assertRaisesRegex(ValueError, "exactly one"):
                module.WindowsVM(0, "tc")

    def test_health_uses_helper_instead_of_competing_for_serial_socket(self):
        module = self.launcher()
        vm = object.__new__(module.WindowsVM)
        vm.running = False
        vm.logger = Mock()
        with patch("socket.socket", side_effect=OSError("QGA already owned by helper")), \
             patch.object(module.subprocess, "run", return_value=types.SimpleNamespace(returncode=0)) as run, \
             patch.object(module.time, "sleep"):
            vm.bootstrap_spin()
        self.assertTrue(vm.running)
        self.assertEqual(run.call_args.args[0], ["/labcontainers-guest", "ping", "2s"])

    def test_failed_probe_does_not_mark_guest_ready(self):
        module = self.launcher()
        vm = object.__new__(module.WindowsVM)
        vm.running = False
        vm.logger = Mock()
        with patch("socket.socket", side_effect=OSError("not ready")), \
             patch.object(module.subprocess, "run", return_value=types.SimpleNamespace(returncode=125)), \
             patch.object(module.time, "sleep"):
            vm.bootstrap_spin()
        self.assertFalse(vm.running)


    def test_helper_timeout_does_not_mark_guest_ready(self):
        module = self.launcher()
        vm = object.__new__(module.WindowsVM)
        vm.running = False
        vm.logger = Mock()
        with patch.object(module.subprocess, "run", side_effect=module.subprocess.TimeoutExpired("ping", 3)), \
             patch.object(module.time, "sleep"):
            vm.bootstrap_spin()
        self.assertFalse(vm.running)


@unittest.skipUnless(os.environ.get("LABCONTAINERS_WINDOWS_HEALTH_LIVE_IMAGE"), "requires prepared Windows VM and KVM")
class WindowsHealthLiveTests(unittest.TestCase):
    def test_wrapper_health_and_guest_execution_share_serial_owner(self):
        from labcontainers import Client, api, containerlab as clab, source

        config = clab.Config(name="windows-health", topology=clab.Topology(nodes={
            "windows": clab.NodeConfig(kind="generic_vm", image=os.environ["LABCONTAINERS_WINDOWS_HEALTH_LIVE_IMAGE"], image_pull_policy="Never"),
        }))
        with Client(labd=os.environ["LABCONTAINERS_LABD"]) as client:
            lab = client.start(api.LabSpec(topology=source(config), nodes={"windows": api.NodeExtension(control="qga")}), ttl_seconds=600)
            result = subprocess.run([
                "docker", "ps", "-q", "--filter", "label=labcontainers.appmana.com/session=" + lab.id,
                "--filter", "label=clab-node-name=windows",
            ], check=True, capture_output=True, text=True, timeout=10)
            containers = result.stdout.split()
            self.assertEqual(len(containers), 1)
            deadline = time.monotonic() + 420
            last_error = None
            while time.monotonic() < deadline:
                try:
                    result = lab.node("windows").exec("cmd.exe", "/c", "ver", timeout_seconds=20)
                    result.check_returncode()
                    self.assertIn(b"Microsoft Windows", result.stdout)
                    break
                except Exception as error:
                    last_error = error
                    time.sleep(2)
            else:
                self.fail(f"guest execution never became ready: {last_error}")
            # Execution has established the helper's persistent serial connection.
            # Wrapper readiness must still succeed without opening a second one.
            deadline = time.monotonic() + 60
            while time.monotonic() < deadline:
                state = subprocess.run(["docker", "inspect", "--format", "{{json .State.Health}}", containers[0]],
                                       check=True, capture_output=True, text=True, timeout=10)
                health = json.loads(state.stdout)
                if health["Status"] == "healthy":
                    break
                time.sleep(1)
            else:
                self.fail(f"guest commands work but wrapper readiness failed: {health}")
            for _ in range(3):
                subprocess.run(["docker", "exec", containers[0], "/labcontainers-guest", "ping", "5s"], check=True, timeout=10)
                result = lab.node("windows").exec("cmd.exe", "/c", "ver", timeout_seconds=20)
                result.check_returncode()
                self.assertIn(b"Microsoft Windows", result.stdout)
            print(f"Windows health and guest commands passed; artifacts: {lab.value.artifact_directory}", flush=True)
