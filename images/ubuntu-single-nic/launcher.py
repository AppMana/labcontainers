#!/usr/bin/env python3
"""Launch a vrnetlab Ubuntu VM with one declared NIC and out-of-band QGA control."""
import argparse
import importlib.util
import logging
import os
from pathlib import Path
import subprocess
import time

spec = importlib.util.spec_from_file_location("ubuntu_launcher", "/launch.py")
ubuntu = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ubuntu)


class SingleNIC(ubuntu.Ubuntu_vm):
    def __init__(self, hostname, username, password, connection_mode):
        # The cloud-provisioning Ubuntu launcher predates vrnetlab-base 0.3.0
        # and extends this optional list. Seed it for compatibility; this
        # Labcontainers wrapper removes the management NIC immediately below.
        self.mgmt_udp_ports = []
        super().__init__(hostname, username, password, 1, connection_mode)
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

    def nic_provision_delay(self):
        # vrnetlab normally waits for data NICs plus eth0 management. This
        # wrapper intentionally has no management interface, so wait only for
        # the declared topology NIC.
        while not Path("/sys/class/net/eth1").exists():
            time.sleep(1)
        self.num_provisioned_nics = 1
        self.highest_provisioned_nic_num = 1


class LabVM(ubuntu.vrnetlab.VR):
    def __init__(self, hostname, username, password, connection_mode):
        super().__init__(username, password)
        self.vms = [SingleNIC(hostname, username, password, connection_mode)]


if __name__ == "__main__":
    # This wrapper deliberately removes vrnetlab's implicit management NIC. Tell
    # vrnetlab that every provisioned interface is topology-owned so it does not
    # wait forever for a nonexistent Docker management interface.
    os.environ.setdefault("VR_MGMT_IS_A_LINK", "true")
    parser = argparse.ArgumentParser()
    parser.add_argument("--hostname", default="ubuntu")
    parser.add_argument("--username", default="sysadmin")
    parser.add_argument("--password", default="sysadmin")
    parser.add_argument("--connection-mode", default="tc")
    parser.add_argument("--trace", action="store_true")
    args = parser.parse_args()
    logging.basicConfig(level=logging.DEBUG if args.trace else logging.INFO)
    subprocess.Popen(["/labcontainers-guest", "serve"])
    reset = Path("/labcontainers-reset-instance")
    if reset.exists():
        for disk in Path("/").glob("*-overlay.qcow2"):
            disk.unlink()
        reset.unlink()
    LabVM(args.hostname, args.username, args.password, args.connection_mode).start()
