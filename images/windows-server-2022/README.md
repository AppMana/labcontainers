# Windows Server 2022 test image

This image is built locally from licensed Microsoft media. The repository does
not redistribute Windows. Packer installs current updates, signed virtio
drivers, QEMU Guest Agent, and build-time WinRM, then generalizes and compacts the
qcow2. Labcontainers creates a disposable copy-on-write overlay per VM.

The design follows the useful parts of Google's image pipeline: bake drivers
and guest control into a generalized base, keep instance identity and test
configuration in first-boot bootstrap, and use an out-of-band guest agent for
lifecycle and diagnostics.

```sh
cd images/windows-server-2022/packer
packer init .
7z x /absolute/path/virtio-win.iso -o/var/tmp/virtio-win
packer build -on-error=abort \
  -var 'iso_url=/absolute/path/windows-server-2022.iso' \
  -var 'iso_checksum=sha256:...' \
  -var 'virtio_win_directory=/var/tmp/virtio-win' \
  -var "windows_password=$LABCONTAINERS_WINDOWS_PASSWORD" .

cd ..
cp packer/output/windows-server-2022/windows-server-2022-labcontainers.qcow2 .
CGO_ENABLED=0 go build -o labcontainers-guest ../../cmd/labcontainers-guest
docker build --build-context labcontainers-common=../common -t labcontainers/windows-server-2022:latest .
```

The runtime VM has no implicit management NIC. QGA travels over virtio-serial;
all Ethernet adapters are created from topology links. Labcontainers data disks
are attached as VirtIO block devices and remain independent of the disposable
OS overlay, so disk-persistence and machine-replacement tests use the same API
as Linux VMs.

NIC count comes from Containerlab's native `CLAB_INTFS`, including zero. Use
contiguous `eth1` through `ethN` endpoints; sparse numbering is rejected rather
than letting vrnetlab add placeholder adapters. Optional `--nics` must match
that count. QEMU's default NIC is explicitly disabled.

Project-specific layers remain in their owning repositories. Pass their
PowerShell entry points through `provisioning_scripts` to bake Kubernetes,
Calico, storage, or cloud-provider prerequisites after the shared VirtIO/QGA
layer. This keeps the generic image reusable without copying provisioning logic
back into Labcontainers.
