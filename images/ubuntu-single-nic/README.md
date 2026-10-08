# Ubuntu declared-NIC VM wrapper

Build `labcontainers-guest` for Linux and place it in this directory, then build
the wrapper after producing `vrnetlab/canonical_ubuntu:jammy-qga` from an Ubuntu
cloud qcow2. The base must have Ubuntu's `qemu-guest-agent` package installed;
the stock Jammy cloud image does not include it.

```sh
CGO_ENABLED=0 go build -o images/ubuntu-single-nic/labcontainers-guest ./cmd/labcontainers-guest
docker build --build-context labcontainers-common=images/common -t labcontainers/vm-ubuntu:jammy images/ubuntu-single-nic
```

For test runners, build the wrapper on the lean base instead. `lean-base.sh`
derives it offline from the `jammy-qga.qcow2` inside
`vrnetlab/canonical_ubuntu:jammy-qga` (pass that file's SHA-256): it purges
snapd and other cloud-platform agents, disables background timers, limits
cloud-init to the NoCloud seed, and sparsifies the disk. It needs
`virt-customize` and `virt-sparsify` (run with `sudo` on Ubuntu, whose kernels
are not world-readable). Package the result with vrnetlab's Ubuntu Dockerfile:

```sh
sudo images/ubuntu-single-nic/lean-base.sh jammy-qga.qcow2 <sha256> /abs/build/jammy-qga-lean.qcow2
docker build -f vrnetlab/ubuntu/docker/Dockerfile --build-arg IMAGE=jammy-qga-lean.qcow2 \
  -t vrnetlab/canonical_ubuntu:jammy-qga-lean /abs/build   # context also holds vrnetlab's launch.py
docker build --build-context labcontainers-common=images/common \
  --build-arg VRNETLAB_IMAGE=vrnetlab/canonical_ubuntu:jammy-qga-lean \
  -t labcontainers/vm-ubuntu:lean images/ubuntu-single-nic
```

The wrapper attaches the root overlay as virtio-blk and adds a virtio balloon
with free page reporting, so memory the guest frees returns to the host. Guests
default to 512 MiB and one vCPU; set `QEMU_MEMORY` (MiB) and `QEMU_SMP` in a
node's native `Env` when a scenario needs more.

`TestLiveVMFootprint` measures boot time and host memory per VM:

```sh
LABCONTAINERS_FOOTPRINT_LIVE=1 LABCONTAINERS_FOOTPRINT_VMS=8 \
  LABCONTAINERS_VM_IMAGE=labcontainers/vm-ubuntu:lean \
  go test ./pkg/client -run '^TestLiveVMFootprint$' -v -count=1 -timeout=20m
```

It writes `footprint.json` (QGA and cloud-init readiness, `systemd-analyze`,
QEMU and wrapper RSS before and after a freed guest allocation) to the
session's artifact directory. `LABCONTAINERS_FOOTPRINT_ENV=QEMU_MEMORY=768,QEMU_SMP=2`
passes node environment unchanged to compare sizes.

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
