#!/usr/bin/env python3
"""Containerlab generic_vm wrapper for the Labcontainers Windows image."""
import argparse
import json
import logging
import os
import re
import socket
import subprocess
import time
from pathlib import Path

import vrnetlab


class WindowsVM(vrnetlab.VM):
    def __init__(self, nics: int, connection_mode: str):
        image = next('/' + name for name in os.listdir('/') if re.search(r'\.qcow2$', name))
        super().__init__('Administrator', '', disk_image=image, ram=8192, smp='4')
        self.num_nics = nics
        self.conn_mode = connection_mode
        self.nic_type = 'virtio-net-pci'
        self.qemu_args.extend([
            '-chardev', 'socket,path=/run/labcontainers-qga.sock,server=on,wait=off,id=qga0',
            '-device', 'virtio-serial-pci,id=serial1',
            '-device', 'virtserialport,chardev=qga0,name=org.qemu.guest_agent.0',
        ])
        for index, disk in enumerate(sorted(Path('/labcontainers-disks').glob('*.raw'))):
            drive = f'labcontainers_disk_{index}'
            serial = 'lc-' + disk.stem[:16]
            self.qemu_args.extend([
                '-drive', f'file={disk},format=raw,if=none,id={drive}',
                '-device', f'virtio-blk-pci,drive={drive},serial={serial}',
            ])

    def gen_mgmt(self):
        # Management remains out-of-band over QGA; every NIC visible to
        # Windows belongs to the declared test topology.
        return []

    def nic_provision_delay(self):
        while not Path('/sys/class/net/eth1').exists():
            time.sleep(1)
        self.num_provisioned_nics = self.num_nics
        self.highest_provisioned_nic_num = self.num_nics

    def bootstrap_spin(self):
        try:
            with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as conn:
                conn.settimeout(2)
                conn.connect('/run/labcontainers-qga.sock')
                token = int(time.time_ns() & ((1 << 52) - 1))
                request = {'execute': 'guest-sync-delimited', 'arguments': {'id': token}}
                conn.sendall(b'\xff' + json.dumps(request).encode() + b'\n')
                data = conn.recv(65536)
                if str(token).encode() not in data:
                    return
                conn.sendall(b'{"execute":"guest-ping"}\n')
                if b'"return"' not in conn.recv(65536):
                    return
            self.running = True
            self.logger.info('Windows QEMU Guest Agent is ready')
        except (OSError, TimeoutError):
            time.sleep(1)


class Windows(vrnetlab.VR):
    def __init__(self, nics: int, connection_mode: str):
        super().__init__('Administrator', '')
        self.vms = [WindowsVM(nics, connection_mode)]


if __name__ == '__main__':
    os.environ.setdefault('VR_MGMT_IS_A_LINK', 'true')
    parser = argparse.ArgumentParser()
    parser.add_argument('--nics', type=int, default=1)
    parser.add_argument('--hostname', default='windows')
    # Containerlab's generic_vm kind supplies the common vrnetlab flags even
    # though QGA, rather than in-band SSH, controls this image.
    parser.add_argument('--username', default='Administrator')
    parser.add_argument('--password', default='')
    parser.add_argument('--connection-mode', default='tc')
    parser.add_argument('--trace', action='store_true')
    args = parser.parse_args()
    logging.basicConfig(level=logging.DEBUG if args.trace else logging.INFO)
    subprocess.Popen(['/labcontainers-guest', 'serve'])
    reset = Path('/labcontainers-reset-instance')
    if reset.exists():
        for disk in Path('/').glob('*-overlay.qcow2'):
            disk.unlink()
        reset.unlink()
    Windows(args.nics, args.connection_mode).start()
