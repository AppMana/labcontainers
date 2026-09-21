#!/usr/bin/env python3
"""Launch a vrnetlab Ubuntu VM with one declared NIC and out-of-band QGA control."""
import argparse
import importlib.util
import logging
from pathlib import Path
import subprocess

spec = importlib.util.spec_from_file_location("ubuntu_launcher", "/launch.py")
ubuntu = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ubuntu)


class SingleNIC(ubuntu.Ubuntu_vm):
    def __init__(self, hostname):
        super().__init__(hostname, "sysadmin", "sysadmin", 1, "tc")
        self.qemu_args.extend([
            "-chardev", "socket,path=/run/labcontainers-qga.sock,server=on,wait=off,id=qga0",
            "-device", "virtio-serial-pci,id=serial1",
            "-device", "virtserialport,chardev=qga0,name=org.qemu.guest_agent.0",
        ])
        for index, disk in enumerate(sorted(Path("/labcontainers-disks").glob("*.raw"))):
            drive = f"labcontainers_disk_{index}"
            serial = "lc-" + disk.stem[:16]
            self.qemu_args.extend([
                "-drive", f"file={disk},format=raw,if=none,id={drive}",
                "-device", f"virtio-blk-pci,drive={drive},serial={serial}",
            ])

    def gen_mgmt(self):
        return []


class LabVM(ubuntu.vrnetlab.VR):
    def __init__(self, hostname):
        super().__init__("sysadmin", "sysadmin")
        self.vms = [SingleNIC(hostname)]


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--hostname", required=True)
    args = parser.parse_args()
    logging.basicConfig(level=logging.INFO)
    subprocess.Popen(["/labcontainers-guest", "serve"])
    reset = Path("/labcontainers-reset-instance")
    if reset.exists():
        for disk in Path("/").glob("*-overlay.qcow2"):
            disk.unlink()
        reset.unlink()
    LabVM(args.hostname).start()
