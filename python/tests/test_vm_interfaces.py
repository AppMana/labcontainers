import importlib.util
import os
from pathlib import Path
from tempfile import TemporaryDirectory
from types import SimpleNamespace
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("vm_interfaces", Path(__file__).resolve().parents[2] / "images/common/interfaces.py")
interfaces = importlib.util.module_from_spec(spec)
spec.loader.exec_module(interfaces)


class VMInterfacesTest(unittest.TestCase):
    def test_native_control_listeners_are_loopback_only(self):
        args = ["qemu-system-x86_64",
                "-chardev socket,id=monitor0,host=::,port=4000,server=on,wait=off",
                "-chardev socket,id=serial0,host=::,port=5000,server=on,wait=off,telnet=on",
                "-drive", "file=/guest.qcow2"]
        vm = SimpleNamespace(qemu_args=args)
        interfaces.isolate_control_listeners(vm)
        self.assertEqual(vm.qemu_args, [arg.replace("host=::", "host=127.0.0.1") for arg in args])
        self.assertIn("host=::", args[1], "must not mutate input list before validating both controls")
        for broken in (args[:2], args + [args[1]], [arg.replace("host=::,", "") for arg in args]):
            vm = SimpleNamespace(qemu_args=broken)
            with self.assertRaises(ValueError):
                interfaces.isolate_control_listeners(vm)
            self.assertEqual(vm.qemu_args, broken)

    def test_native_count_has_no_default_nic(self):
        with patch.dict(os.environ, {}, clear=True):
            self.assertEqual(interfaces.declared_nics(), 0)
        for count in (0, 1, 3):
            with patch.dict(os.environ, {"CLAB_INTFS": str(count)}):
                self.assertEqual(interfaces.declared_nics(), count)
                self.assertEqual(interfaces.declared_nics(count), count)
                with self.assertRaises(ValueError):
                    interfaces.declared_nics(count + 1)
        for value in ("-1", "not-a-count"):
            with patch.dict(os.environ, {"CLAB_INTFS": value}):
                with self.assertRaises(ValueError):
                    interfaces.declared_nics()

    def test_zero_and_multiple_declared_interfaces(self):
        for count in (0, 1, 3):
            with TemporaryDirectory() as tmp:
                root = Path(tmp)
                (root / "lo").touch()
                for i in range(1, count + 1):
                    (root / f"eth{i}").touch()
                vm = SimpleNamespace(num_nics=count)
                interfaces.wait_for_interfaces(vm, root, timeout=0)
                self.assertEqual(vm.num_provisioned_nics, count)
                self.assertEqual(vm.highest_provisioned_nic_num, count)

    def test_missing_or_undeclared_interfaces_fail_closed(self):
        for found in ([], ["eth0"], ["eth2"], ["eth1", "eth2"]):
            with TemporaryDirectory() as tmp:
                root = Path(tmp)
                for name in found:
                    (root / name).touch()
                with self.assertRaises((ValueError, TimeoutError)):
                    interfaces.wait_for_interfaces(SimpleNamespace(num_nics=1), root, timeout=0)
