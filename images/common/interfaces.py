"""Containerlab endpoint accounting without vrnetlab's management-NIC default."""

import os
from pathlib import Path
import time


def isolate_control_listeners(vm):
    """Keep vrnetlab's existing console clients on wrapper loopback only.

    Preserve the upstream command syntax and ports; do not expose QEMU control
    on any simulated endpoint. Fail closed if an upstream change prevents us
    from locating either known TCP control socket.
    """
    arguments = list(vm.qemu_args)
    for identity in ("monitor0", "serial0"):
        matches = [i for i, arg in enumerate(arguments) if f",id={identity}," in arg]
        if len(matches) != 1:
            raise ValueError(f"cannot isolate native QEMU {identity} listener")
        index = matches[0]
        fields = arguments[index].split(",")
        hosts = [i for i, field in enumerate(fields) if field.startswith("host=")]
        if len(hosts) != 1:
            raise ValueError(f"native QEMU {identity} has no unique bind address")
        fields[hosts[0]] = "host=127.0.0.1"
        arguments[index] = ",".join(fields)
    vm.qemu_args = arguments


def declared_nics(requested=None):
    """Use Containerlab's native endpoint count, never an inferred mgmt NIC."""
    count = int(os.environ.get("CLAB_INTFS", "0"))
    if count < 0 or (requested is not None and requested != count):
        raise ValueError("--nics must match the native CLAB_INTFS endpoint count")
    return count


def wait_for_interfaces(vm, root=Path("/sys/class/net"), timeout=120):
    """Wait for exactly eth1..ethN before upstream gen_nics runs.

    Sparse indices would cause vrnetlab to create socket-backed placeholder
    NICs. Reject those rather than introduce undeclared guest adapters.
    """
    expected = {f"eth{i}" for i in range(1, vm.num_nics + 1)}
    deadline = time.monotonic() + timeout
    while True:
        found = {p.name for p in root.glob("eth*")}
        unexpected = found - expected
        if unexpected:
            raise ValueError(f"undeclared or unsupported VM endpoints: {sorted(unexpected)}; expected {sorted(expected)}")
        if found == expected:
            vm.num_provisioned_nics = vm.num_nics
            vm.highest_provisioned_nic_num = vm.num_nics
            return
        if time.monotonic() >= deadline:
            raise TimeoutError(f"declared VM endpoints did not arrive: {sorted(expected - found)}")
        time.sleep(0.1)


def virtio_root_disk(vm):
    """Attach vrnetlab's root overlay as virtio-blk instead of emulated IDE.

    Firmware and guest both read the root disk faster over virtio, and the
    guest's discards shrink the overlay. Fail closed unless exactly one
    native IDE overlay drive is present.
    """
    arguments = list(vm.qemu_args)
    matches = [i for i, arg in enumerate(arguments)
               if i > 0 and arguments[i - 1] == "-drive"
               and arg.startswith("if=ide,file=") and arg.endswith("-overlay.qcow2")
               and "," not in arg[len("if=ide,file="):]]
    if len(matches) != 1:
        raise ValueError("cannot find the unique native IDE root overlay drive")
    index = matches[0]
    overlay = arguments[index][len("if=ide,file="):]
    arguments[index] = f"if=none,id=lc-root,file={overlay},discard=unmap"
    arguments[index + 1:index + 1] = ["-device", "virtio-blk-pci,drive=lc-root,bootindex=0"]
    vm.qemu_args = arguments


def free_page_reporting(vm):
    """Return memory the guest frees to the host instead of keeping it resident."""
    vm.qemu_args.extend(["-device", "virtio-balloon-pci,id=lc-balloon,free-page-reporting=on"])
