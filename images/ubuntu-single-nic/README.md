# Ubuntu declared-NIC VM wrapper

Build `labcontainers-guest` for Linux and place it in this directory, then build
the wrapper after producing `vrnetlab/canonical_ubuntu:jammy-qga` from an Ubuntu
cloud qcow2. The base must have Ubuntu's `qemu-guest-agent` package installed;
the stock Jammy cloud image does not include it.

```sh
CGO_ENABLED=0 go build -o images/ubuntu-single-nic/labcontainers-guest ./cmd/labcontainers-guest
docker build --build-context labcontainers-common=images/common -t labcontainers/vm-ubuntu:jammy images/ubuntu-single-nic
```

The wrapper has no management NIC. Zero or more Ethernet interfaces are supplied
by the Containerlab topology; commands use QEMU Guest Agent over virtio-serial.
The native `CLAB_INTFS` endpoint count controls NIC count. Endpoints must be
contiguous `eth1` through `ethN`; sparse numbering is rejected because vrnetlab
would otherwise generate undeclared placeholder adapters. `--nics`, if supplied,
must match the topology count. QEMU's implicit NIC is disabled even at zero NICs.
QEMU monitor and serial-console listeners bind only to the wrapper's loopback;
they are not exposed on the simulated data links. QGA uses its Unix socket.
The bundled network policy keeps unmatched topology NICs optional and offline;
tests that need guest networking bind a node-specific cloud-init network config
over `/extra-network.yaml`.

To qualify an explicitly prepared candidate without replacing a shared image:

```sh
LABCONTAINERS_NICS_LIVE_IMAGE=labcontainers/vm-ubuntu:native-nics \
  python -m unittest discover -s python/tests -p test_vm_nics_live.py -v
```

Run from the repository root with the SDK installed and the daemon built. The
test boots zero- and two-NIC VMs, observes guest adapters and IPv4/IPv6 default
routes through QGA, checks the wrapper's actual TCP listener addresses, then
cleans up its sessions. It does not qualify Kubernetes
or the Windows image. The historical directory name remains for compatibility.
