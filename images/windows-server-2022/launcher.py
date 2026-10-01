#!/usr/bin/env python3
"""Containerlab generic_vm wrapper for the Labcontainers Windows image."""
import argparse
import logging
import os
import subprocess
import time
from pathlib import Path

import vrnetlab
from interfaces import declared_nics, isolate_control_listeners, wait_for_interfaces


class WindowsVM(vrnetlab.VM):
    def __init__(self, nics: int, connection_mode: str):
        # vrnetlab reuses the base's existing overlay on restart. Selecting an
        # overlay here creates another overlay and makes disk identity depend
        # on directory enumeration order after every stop/start.
        bases = [name for name in os.listdir('/')
                 if name.endswith('.qcow2') and not name.endswith('-overlay.qcow2')]
        if len(bases) != 1:
            raise ValueError(f'expected exactly one base qcow2, found {sorted(bases)}')
        image = '/' + bases[0]
        super().__init__('Administrator', '', disk_image=image, ram=8192, smp='4')
        isolate_control_listeners(self)
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
        return ['-nic', 'none']

    def nic_provision_delay(self):
        wait_for_interfaces(self)

    def bootstrap_spin(self):
        try:
            # Only the helper owns QGA. Its ping serializes with execution;
            # a second direct socket client competes with its persistent link.
            result = subprocess.run(
                ['/labcontainers-guest', 'ping', '2s'],
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=3,
            )
            if result.returncode == 0:
                self.running = True
                self.logger.info('Windows QEMU Guest Agent is ready')
                return
        except (OSError, subprocess.TimeoutExpired):
            pass
        time.sleep(1)


class Windows(vrnetlab.VR):
    def __init__(self, nics: int, connection_mode: str):
        super().__init__('Administrator', '')
        self.vms = [WindowsVM(nics, connection_mode)]


if __name__ == '__main__':
    os.environ.setdefault('VR_MGMT_IS_A_LINK', 'true')
    parser = argparse.ArgumentParser()
    parser.add_argument('--nics', type=int, default=None)
    parser.add_argument('--hostname', default='windows')
    # Containerlab's generic_vm kind supplies the common vrnetlab flags even
    # though QGA, rather than in-band SSH, controls this image.
    parser.add_argument('--username', default='Administrator')
    parser.add_argument('--password', default='')
    parser.add_argument('--connection-mode', default='tc')
    parser.add_argument('--trace', action='store_true')
    args = parser.parse_args()
    nics = declared_nics(args.nics)
    logging.basicConfig(level=logging.DEBUG if args.trace else logging.INFO)
    subprocess.Popen(['/labcontainers-guest', 'serve'])
    reset = Path('/labcontainers-reset-instance')
    if reset.exists():
        for disk in Path('/').glob('*-overlay.qcow2'):
            disk.unlink()
        reset.unlink()
    Windows(nics, args.connection_mode).start()
