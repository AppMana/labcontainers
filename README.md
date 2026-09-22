# Labcontainers

Crash tests use `Node.Crash()` (Python/JavaScript `node.crash()`) or the `CRASH`
lifecycle action. It resolves one running container using topology and node
labels and sends SIGKILL, killing its QEMU process without guest shutdown.
Attached disks persist. `PowerOff()` retains Containerlab stop semantics.

Timelines accept `wait_exec`: a bounded guest predicate with an expected exit
code and optional stdout/stderr substring matches. A match permits the next
action; timeout aborts the timeline. Ordinary timeline `exec` now aborts on
nonzero exit status. Observation and crash are separate operations: polling
cannot guarantee a transient state remains active at the instant of kill.
Exact application boundaries require an explicit barrier. Events record guest
command exits and lifecycle completion.

The manual `VM runtime qualification` workflow runs the Windows QGA/NTFS/crash
test on a dedicated runner labelled `self-hosted`, `linux`, `x64`, `kvm`, and
`seaweedfs-lab`. Set Actions variable `LABCONTAINERS_WINDOWS_IMAGE` to a preloaded
image's `repository@sha256:...` reference. For local reproduction run `make build`
and `LABCONTAINERS_WINDOWS_LIVE=1 go test ./pkg/client -run '^TestLiveWindows$' -v -count=1 -timeout=18m`.
The ordinary unit suite skips this privileged runtime test.

Labcontainers is a Testcontainers-style API for isolated container and virtual
machine test networks. It keeps Containerlab as the topology and dataplane
engine, and adds test-session ownership, VM control without a management NIC,
fault scheduling, cleanup, artifacts, and language-neutral APIs.

It is not a Containerlab fork. Labcontainers currently pins the stock
Containerlab `v0.79.0` CLI and accepts unmodified `.clab.yml` topology files.

## Status

The Go, Python, and Node.js SDKs share a versioned gRPC API. Implemented runtime
operations include:

- private per-suite `labd` children over a `0600` Unix socket;
- isolated topology validation and ownership labels;
- create, inspect, destroy, keep, resume, and expired-session scavenging;
- container and QEMU Guest Agent exec/file transfer;
- abrupt power-off, start, restart, and fresh node replacement;
- link state and Containerlab netem faults;
- deterministic fault/exec/lifecycle timelines with rollback;
- clean CAPI `LabCluster`, `LabMachine`, `LabMachineTemplate`, and
  `LabImportedControlPlane` contracts.

Install the CAPI contract with `labctl capi-crds | kubectl apply -f -`.

Group partitions requiring host bridge filters are rejected explicitly until
that backend lands; no best-effort partition is reported as successful.

## Prerequisites

- Linux x86-64
- Docker and KVM/QEMU for VM nodes
- Containerlab exactly `v0.79.0`
- non-interactive scoped `sudo` access for Containerlab network operations

Run `labctl doctor` before a suite. Labcontainers never creates Containerlab's
implicit management network under its default policy: every non-bridge node
must resolve to `network-mode: none`, published ports and external link types
are rejected, and `mgmt.skip-when-unused` is added to the private topology.

Install the daemon and diagnostic CLI once for all language SDKs:

```sh
go install github.com/appmana/labcontainers/cmd/labd@v0.2.0-alpha.2
go install github.com/appmana/labcontainers/cmd/labctl@v0.2.0-alpha.2
```

`labd` must be on `PATH`, or its path can be passed as Go's `LabdPath`,
Python's `labd=`, or Node.js's `{labd: ...}` launch option.

## Go

```go
ctx := context.Background()
c, err := client.Launch(ctx, client.Options{})
if err != nil { log.Fatal(err) }
defer c.Close()

lab, err := c.Start(ctx, &labcontainersv1.LabSpec{
    Topology: &labcontainersv1.TopologySource{
        Source: &labcontainersv1.TopologySource_Path{Path: "basic.clab.yml"},
    },
}, 30*time.Minute)
if err != nil { log.Fatal(err) }

result, err := lab.Node("client").Exec(ctx, "ping", "-c", "1", "192.0.2.2")
```

## Python

```python
from labcontainers import Client, api

with Client() as client:
    lab = client.start(api.LabSpec(
        topology=api.TopologySource(path="basic.clab.yml")))
    result = lab.node("client").exec("ping", "-c", "1", "192.0.2.2")
    result.check_returncode()
```

Python can be installed directly from a Git checkout:

```sh
python -m pip install 'git+https://github.com/AppMana/labcontainers.git@v0.2.0-alpha.2'
```

## Node.js

```js
const {Client} = require('@appmana/labcontainers');

const client = await Client.launch();
try {
  const lab = await client.start({
    topology: {path: 'basic.clab.yml'},
  });
  const result = await lab.node('client').exec(
    ['ping', '-c', '1', '192.0.2.2']);
  if (result.exitCode !== 0) throw new Error(result.stderr.toString());
} finally {
  await client.close();
}
```

The npm package is also Git-installable without a publish step:

```sh
npm install 'git+https://github.com/AppMana/labcontainers.git#v0.2.0-alpha.2'
```

Use `session.keep()` only for debugging. It returns a resume token in the raw
API and extends the session lease; ordinary test sessions are destroyed when
their owning SDK closes or dies.

## VM nodes

`images/ubuntu-single-nic` contains the vrnetlab wrapper. The guest has only
topology-declared Ethernet interfaces. The host reaches QEMU Guest Agent over a
virtio-serial socket, so commands remain available while the network is cut.
Windows images use the same protocol but must be built from user-supplied,
licensed media.

`images/windows-server-2022` contains a Packer/QEMU pipeline for a reusable
Windows Server 2022 base image. It installs signed VirtIO drivers, QEMU Guest
Agent, build-time WinRM, and current Windows updates before running Sysprep. The
runtime container starts each test from a disposable qcow2 overlay and exposes
only topology-declared NICs. See that directory's README for the build command
and ISO requirements.

## Development

```sh
make generate
make test
make build
```

Containerlab is a separate BSD-3-Clause dependency. Labcontainers source is
Apache-2.0 licensed.
