"""Containerlab endpoint accounting without vrnetlab's management-NIC default."""

import os
from pathlib import Path
import time


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
