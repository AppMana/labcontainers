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
docker build -t labcontainers/windows-server-2022:latest .
```

The runtime VM has no implicit management NIC. QGA travels over virtio-serial;
all Ethernet adapters are created from topology links. Labcontainers data disks
are attached as VirtIO block devices and remain independent of the disposable
OS overlay, so disk-persistence and machine-replacement tests use the same API
as Linux VMs.

For Linux-side `labcontainers-guest` changes, a helper-only layer avoids
rebuilding Windows or changing its qcow2. From a clean, committed repository
root, select an existing local base and a new output tag:

```sh
lab_base=labcontainers/windows-server-2022:your-existing-base
lab_output=labcontainers/windows-server-2022:your-new-helper-tag
lab_context=$(mktemp -d /tmp/labcontainers-helper.XXXXXXXX)
lab_revision=$(git rev-parse HEAD)
docker image inspect "$lab_base" --format '{{.Id}}'
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -o "$lab_context/labcontainers-guest" ./cmd/labcontainers-guest
lab_helper_sha=$(sha256sum "$lab_context/labcontainers-guest" | cut -d' ' -f1)
docker build --pull=false --network=none \
  -f images/windows-server-2022/Dockerfile.helper \
  --build-arg BASE_IMAGE="$lab_base" \
  --build-arg SOURCE_REVISION="$lab_revision" \
  --build-arg HELPER_SHA256="$lab_helper_sha" \
  -t "$lab_output" "$lab_context"
docker image inspect "$lab_output" --format '{{.Id}}'
```

Retain the base/output image IDs, helper hash, `go version -m` output and test
results. Use the new image only for new lab sessions: replacing a helper inside
an active test would break its evidence chain. This changes the Linux host-side
QGA adapter, not the Windows QEMU Guest Agent service, drivers or OS image.

Project-specific layers remain in their owning repositories. Pass their
PowerShell entry points through `provisioning_scripts` to bake Kubernetes,
Calico, storage, or cloud-provider prerequisites after the shared VirtIO/QGA
layer. This keeps the generic image reusable without copying provisioning logic
back into Labcontainers.
