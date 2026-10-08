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


def netns_interfaces(dev=Path("/proc/net/dev")):
    """Interfaces of this process's network namespace.

    /proc/net follows the reader's namespace. /sys/class/net does not when a
    rootless runtime could not mount a fresh sysfs and bind-mounted its own.
    """
    names = set()
    for line in dev.read_text().splitlines()[2:]:
        name, sep, _ = line.partition(":")
        if sep:
            names.add(name.strip())
    return names


def wait_for_interfaces(vm, list_interfaces=netns_interfaces, timeout=120):
    """Wait for exactly eth1..ethN before upstream gen_nics runs.

    Sparse indices would cause vrnetlab to create socket-backed placeholder
    NICs. Reject those rather than introduce undeclared guest adapters.
    """
    expected = {f"eth{i}" for i in range(1, vm.num_nics + 1)}
    deadline = time.monotonic() + timeout
    while True:
        found = {name for name in list_interfaces() if name.startswith("eth")}
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


def use_netns_interface_view(list_interfaces=netns_interfaces):
    """Answer vrnetlab's /sys/class/net existence checks from this namespace.

    Under a rootless runtime in an unprivileged pod, the container cannot
    mount its own sysfs (the pod's is partly masked), so the runtime
    bind-mounts the pod's, which lists the pod's interfaces. vrnetlab only
    tests /sys/class/net/ethN for existence; QEMU, tc and ip use netlink.
    """
    exists = os.path.exists
    prefix = "/sys/class/net/"

    def netns_exists(path):
        text = os.fspath(path)
        if text.startswith(prefix) and "/" not in text[len(prefix):]:
            return text[len(prefix):] in list_interfaces()
        return exists(path)

    os.path.exists = netns_exists


def die_with_launcher(vm):
    """Make QEMU exit when the launcher (the container's PID 1) is killed.

    A runtime that shares the host PID namespace and has no cgroup (rootless
    Podman in an unprivileged pod) can only signal PID 1, so a crash or
    removal would otherwise leave QEMU running with the disks open.
    """
    if vm.qemu_args[0].startswith("qemu-system-"):
        vm.qemu_args[0] = "setpriv --pdeathsig KILL " + vm.qemu_args[0]
    elif not vm.qemu_args[0].startswith("setpriv --pdeathsig KILL qemu-system-"):
        raise ValueError(f"unexpected QEMU command {vm.qemu_args[0]!r}")


def die_with_parent():
    """Popen preexec_fn: SIGKILL this child when its parent exits."""
    import ctypes
    import signal
    PR_SET_PDEATHSIG = 1
    if ctypes.CDLL(None, use_errno=True).prctl(PR_SET_PDEATHSIG, signal.SIGKILL) != 0:
        raise OSError(ctypes.get_errno(), "prctl(PR_SET_PDEATHSIG)")


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
