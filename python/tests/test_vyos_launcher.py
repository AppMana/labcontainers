import importlib.util
from pathlib import Path
import sys
import types
import unittest
from unittest.mock import Mock, patch


class VyOSLauncherTests(unittest.TestCase):
    def load(self):
        class SerialQGA:
            def __init__(self, nics, mode):
                self.qemu_args = []
                self.running = False
                self.tn = Mock()

            def bootstrap_spin(self):
                pass

        spec = importlib.util.spec_from_file_location("vyos_launcher", Path(__file__).resolve().parents[2] / "images/vyos/launcher.py")
        module = importlib.util.module_from_spec(spec)
        with patch.dict(sys.modules, vrnetlab=types.SimpleNamespace(VR=object),
                        interfaces=types.SimpleNamespace(declared_nics=Mock()),
                        qga_vm=types.SimpleNamespace(WindowsVM=SerialQGA)):
            spec.loader.exec_module(module)
        return module

    def test_control_bootstrap_never_adds_a_network(self):
        vm = self.load().VyOSVM(3, "tc")
        self.assertFalse(any("netdev" in arg or "hostfwd" in arg for arg in vm.qemu_args))
        vm.tn.expect.return_value = (2, None, b"")
        vm.bootstrap_spin()
        command = vm.tn.write.call_args.args[0]
        self.assertIn(b"dpkg -i", command)
        self.assertNotIn(b"apt", command)
        self.assertNotIn(b"route", command)
        self.assertNotIn(b"ssh", command)
        vm.bootstrap_spin()
        vm.tn.write.assert_called_once()

    def test_ready_guest_does_not_reinstall_agent(self):
        vm = self.load().VyOSVM(0, "tc")
        vm.running = True
        vm.bootstrap_spin()
        vm.tn.expect.assert_not_called()
