# Ubuntu single-NIC VM wrapper

Build `labcontainers-guest` for Linux and place it in this directory, then build
the wrapper after producing `vrnetlab/canonical_ubuntu:jammy` from an Ubuntu
cloud qcow2:

```sh
CGO_ENABLED=0 go build -o images/ubuntu-single-nic/labcontainers-guest ./cmd/labcontainers-guest
docker build -t labcontainers/vm-ubuntu:jammy images/ubuntu-single-nic
```

The wrapper has no management NIC. Its only Ethernet interface is supplied by
the Containerlab topology; commands use QEMU Guest Agent over virtio-serial.
