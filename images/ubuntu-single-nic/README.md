# Ubuntu single-NIC VM wrapper

Build `labcontainers-guest` for Linux and place it in this directory, then build
the wrapper after producing `vrnetlab/canonical_ubuntu:jammy-qga` from an Ubuntu
cloud qcow2. The base must have Ubuntu's `qemu-guest-agent` package installed;
the stock Jammy cloud image does not include it.

```sh
CGO_ENABLED=0 go build -o images/ubuntu-single-nic/labcontainers-guest ./cmd/labcontainers-guest
docker build -t labcontainers/vm-ubuntu:jammy images/ubuntu-single-nic
```

The wrapper has no management NIC. Its only Ethernet interface is supplied by
the Containerlab topology; commands use QEMU Guest Agent over virtio-serial.
The bundled network policy keeps unmatched topology NICs optional and offline;
tests that need guest networking bind a node-specific cloud-init network config
over `/extra-network.yaml`.
