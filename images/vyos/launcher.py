#!/usr/bin/env python3
"""Installed VyOS, declared NICs, and the existing serial-QGA VM runtime.

The native Debian QGA package is supplied on read-only offline media. Console
bootstrap installs only that control agent; it never configures NICs or routes.
"""
import argparse
import logging
import os
import subprocess

import vrnetlab
from interfaces import declared_nics
from qga_vm import WindowsVM as SerialQGAVM


class VyOSVM(SerialQGAVM):
    def __init__(self, nics, connection_mode):
        super().__init__(nics, connection_mode)
        # VyOS installs both BIOS and EFI bootloaders. Use the runtime's native
        # BIOS; this lane does not claim UEFI/Secure Boot qualification.
        self.qemu_args.extend([
            "-drive", "file=/qga-tools.iso,format=raw,media=cdrom,readonly=on",
        ])
        self.agent_install_started = False

    def bootstrap_spin(self):
        # Reuse the same helper/serial ownership, overlay selection and native
        # endpoint mapping as the existing Windows QGA wrapper.
        super().bootstrap_spin()
        if self.running or self.agent_install_started:
            return
        index, _, _ = self.tn.expect([
            rb"login:\s*$", rb"Password:\s*$", rb"vyos@[^\r\n]+\$\s*$",
        ], 1)
        if index == 0:
            self.tn.write(b"vyos\r")
        elif index == 1:
            self.tn.write(b"vyos\r")
        elif index == 2:
            self.agent_install_started = True
            self.tn.write(
                b"sudo mkdir -p /run/labcontainers-media && "
                b"sudo mount -o ro /dev/sr0 /run/labcontainers-media && "
                b"sudo dpkg -i /run/labcontainers-media/*.deb && "
                b"sudo systemctl start qemu-guest-agent\r"
            )


class VyOS(vrnetlab.VR):
    def __init__(self, nics, connection_mode):
        super().__init__("vyos", "vyos")
        self.vms = [VyOSVM(nics, connection_mode)]


if __name__ == "__main__":
    os.environ.setdefault("VR_MGMT_IS_A_LINK", "true")
    parser = argparse.ArgumentParser()
    parser.add_argument("--nics", type=int, default=None)
    parser.add_argument("--hostname", default="vyos")
    parser.add_argument("--username", default="vyos")
    parser.add_argument("--password", default="vyos")
    parser.add_argument("--connection-mode", default="tc")
    parser.add_argument("--trace", action="store_true")
    args = parser.parse_args()
    logging.basicConfig(level=logging.DEBUG if args.trace else logging.INFO)
    subprocess.Popen(["/labcontainers-guest", "serve"])
    VyOS(declared_nics(args.nics), args.connection_mode).start()
