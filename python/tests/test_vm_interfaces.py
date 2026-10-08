import importlib.util
import os
from pathlib import Path
from tempfile import TemporaryDirectory
from types import SimpleNamespace
import unittest
from unittest.mock import patch
import xml.etree.ElementTree as ET

spec = importlib.util.spec_from_file_location("vm_interfaces", Path(__file__).resolve().parents[2] / "images/common/interfaces.py")
interfaces = importlib.util.module_from_spec(spec)
spec.loader.exec_module(interfaces)


class VMInterfacesTest(unittest.TestCase):
    def test_windows_image_uses_utc_rtc_convention(self):
        answer = Path(__file__).resolve().parents[2] / "images/windows-server-2022/packer/autounattend.xml.pkrtpl"
        root = ET.parse(answer).getroot()
        zones = root.findall(".//{urn:schemas-microsoft-com:unattend}TimeZone")
        self.assertEqual([zone.text for zone in zones], ["UTC"])

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

    def test_netns_interfaces_parse_proc_net_dev(self):
        with TemporaryDirectory() as tmp:
            dev = Path(tmp) / "dev"
            dev.write_text("Inter-|   Receive\n face |bytes\n    lo: 0 0\n  eth1: 1 2\neth10: 3 4\n")
            self.assertEqual(interfaces.netns_interfaces(dev), {"lo", "eth1", "eth10"})

    def test_sysfs_net_existence_follows_this_namespace(self):
        original = os.path.exists
        try:
            interfaces.use_netns_interface_view(lambda: {"lo", "eth1"})
            self.assertTrue(os.path.exists("/sys/class/net/eth1"))
            self.assertFalse(os.path.exists("/sys/class/net/eth0"))
            self.assertFalse(os.path.exists("/sys/class/net/eth2"))
            self.assertEqual(os.path.exists("/sys/class/net/eth1/address"), original("/sys/class/net/eth1/address"))
            self.assertTrue(os.path.exists(__file__))
        finally:
            os.path.exists = original

    def test_root_overlay_moves_to_virtio_blk(self):
        args = ["qemu-system-x86_64", "-m", "512",
                "-drive", "if=ide,file=/jammy-qga-overlay.qcow2",
                "-cdrom", "/cloud_init.iso"]
        vm = SimpleNamespace(qemu_args=args)
        interfaces.virtio_root_disk(vm)
        self.assertEqual(vm.qemu_args, ["qemu-system-x86_64", "-m", "512",
                                        "-drive", "if=none,id=lc-root,file=/jammy-qga-overlay.qcow2,discard=unmap",
                                        "-device", "virtio-blk-pci,drive=lc-root,bootindex=0",
                                        "-cdrom", "/cloud_init.iso"])
        self.assertEqual(args[4], "if=ide,file=/jammy-qga-overlay.qcow2", "must not mutate the input list")

    def test_root_overlay_rewrite_fails_closed(self):
        overlay = ["-drive", "if=ide,file=/a-overlay.qcow2"]
        for broken in ([], overlay + overlay, ["-drive", "if=ide,file=/a.qcow2"],
                       ["-drive", "if=ide,file=/a-overlay.qcow2,cache=none"], ["-hda", "if=ide,file=/a-overlay.qcow2"]):
            vm = SimpleNamespace(qemu_args=list(broken))
            with self.assertRaises(ValueError):
                interfaces.virtio_root_disk(vm)
            self.assertEqual(vm.qemu_args, broken)

    def test_qemu_dies_with_the_launcher(self):
        vm = SimpleNamespace(qemu_args=["qemu-system-x86_64", "-m", "512"])
        interfaces.die_with_launcher(vm)
        self.assertEqual(vm.qemu_args, ["setpriv --pdeathsig KILL qemu-system-x86_64", "-m", "512"])
        interfaces.die_with_launcher(vm)
        self.assertEqual(vm.qemu_args[0], "setpriv --pdeathsig KILL qemu-system-x86_64")
        with self.assertRaises(ValueError):
            interfaces.die_with_launcher(SimpleNamespace(qemu_args=["/usr/bin/kvm"]))

    def test_child_is_killed_with_its_parent(self):
        import subprocess, signal, sys, time
        script = "import subprocess,sys,time; p=subprocess.Popen(['sleep','60'], preexec_fn=__import__('interfaces').die_with_parent); print(p.pid, flush=True); time.sleep(60)"
        env = dict(os.environ, PYTHONPATH=str(Path(__file__).resolve().parents[2] / "images/common"))
        parent = subprocess.Popen([sys.executable, "-c", script], stdout=subprocess.PIPE, text=True, env=env)
        child = int(parent.stdout.readline())
        parent.send_signal(signal.SIGKILL)
        parent.wait()
        for _ in range(50):
            try:
                os.kill(child, 0)
            except ProcessLookupError:
                break
            time.sleep(0.05)
        else:
            self.fail("child outlived its killed parent")

    def test_free_page_reporting_device(self):
        vm = SimpleNamespace(qemu_args=["qemu-system-x86_64"])
        interfaces.free_page_reporting(vm)
        self.assertEqual(vm.qemu_args, ["qemu-system-x86_64", "-device", "virtio-balloon-pci,id=lc-balloon,free-page-reporting=on"])

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
                interfaces.wait_for_interfaces(vm, lambda: {p.name for p in root.iterdir()}, timeout=0)
                self.assertEqual(vm.num_provisioned_nics, count)
                self.assertEqual(vm.highest_provisioned_nic_num, count)

    def test_missing_or_undeclared_interfaces_fail_closed(self):
        for found in ([], ["eth0"], ["eth2"], ["eth1", "eth2"]):
            with TemporaryDirectory() as tmp:
                root = Path(tmp)
                for name in found:
                    (root / name).touch()
                with self.assertRaises((ValueError, TimeoutError)):
                    interfaces.wait_for_interfaces(SimpleNamespace(num_nics=1), lambda: {p.name for p in root.iterdir()}, timeout=0)
