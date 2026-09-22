# Labcontainers

Crash tests use `Node.Crash()` (Python/JavaScript `node.crash()`) or the `CRASH`
lifecycle action. It resolves one running container using topology and node
labels and sends SIGKILL, killing its QEMU process without guest shutdown.
Attached disks persist. `PowerOff()` retains Containerlab stop semantics.

Current lifecycle reconciliation can recreate a VM container and its root disk
after start/restart; only explicitly attached persistent disks are covered by
the persistence guarantee. Explicit `Plan`/`Apply` topology updates report and
recheck native impact, but legacy `Start`/`Restart` have not yet been migrated to
that approval flow. Do not treat a successful `Start` as
proof that container identity or root-disk contents were preserved.

VM teardown can recreate Containerlab veth links. Labcontainers journals Linux
peer-container bridge memberships before crash/stop/restart/replacement and
restores them before `Start` returns, so a caller does not need to reconnect
the switch port. Recovery is restricted to the same running, session-owned
peer container IDs; missing peers or failed reattachment fail the operation
and retain its retry journal. This preserves bridge membership, not arbitrary
port configuration such as VLAN filters, qdiscs, or routes, nor configuration
inside a replaced switch. Host bridges are not modified by this recovery.
The current journal includes all bridge members on surviving Linux peers:
overlapping multi-node outages may require starting both nodes and retrying
the first failed `Start` once both cables exist. A peer replacement can also
block an outstanding journal. These cases fail closed; simultaneous recovery
without retries is not yet qualified.

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

The same workflow runs the Linux VM bridge-restart regression with a preloaded
`LABCONTAINERS_VM_IMAGE` Actions variable (`repository@sha256:...`). Locally:
`make build` then `LABCONTAINERS_VM_LIVE=1 go test ./pkg/client -run '^TestLiveVMCrashRestoresRuntimeBridgeMembership$' -v -count=1 -timeout=12m`.
`LABCONTAINERS_VM_IMAGE` can override the local Ubuntu image and
`LABCONTAINERS_LABD` can select a previously built daemon for RED/GREEN testing.

Labcontainers is a Testcontainers-style API for isolated container and virtual
machine test networks. It keeps Containerlab as the topology and dataplane
engine, and adds test-session ownership, VM control without a management NIC,
fault scheduling, cleanup, artifacts, and language-neutral APIs.

It is not a Containerlab fork. Labcontainers currently pins the stock
Containerlab `v0.79.0`. Tests construct native Go objects or Python objects
generated from Containerlab's JSON Schema. The SDK serializes them internally;
tests do not need YAML strings or files. Existing topology files remain accepted
for interoperability with the Containerlab CLI.

The core SDK has no Kubernetes requirement. A VM, switch, container, or explicit
external connection uses the underlying Containerlab node/link types. Kubernetes
fixtures are a separate specialization, not a replacement topology or VM API.

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
- Go 1.26 or newer for the native Go API (required by Containerlab)
- non-interactive scoped `sudo` access for Containerlab network operations

Run `labctl doctor` before a suite. Labcontainers never creates Containerlab's
implicit management network under its default policy: omitted node network mode
becomes `none`; explicit non-isolated modes, published ports, and external links
(including short-form host/management links and borrowed host bridges) require opt-in. There is no default
WAN or egress connection. `mgmt.skip-when-unused` is set on the private topology.
An external-access opt-in permits explicitly declared connections; it does not
enable implicit networks or disable runtime checks on the other isolated nodes.
Runtime Docker checks alone do not certify guest NICs or application reachability;
network-sensitive tests must also cut their declared paths and probe from guests.

Install the daemon and diagnostic CLI once for all language SDKs:

```sh
go install github.com/appmana/labcontainers/cmd/labd@v0.2.0-alpha.2
go install github.com/appmana/labcontainers/cmd/labctl@v0.2.0-alpha.2
```

`labd` must be on `PATH`, or its path can be passed as Go's `LabdPath`,
Python's `labd=`, or Node.js's `{labd: ...}` launch option.

## Go

```go
// Native imports, not a second Labcontainers topology model:
// clab "github.com/appmana/labcontainers/pkg/containerlab"
// "github.com/srl-labs/containerlab/core"
// "github.com/srl-labs/containerlab/types"
// "github.com/srl-labs/containerlab/links"

ctx := context.Background()
c, err := client.Launch(ctx, client.Options{})
if err != nil { log.Fatal(err) }
defer c.Close()

topology, err := clab.Source(&core.Config{
    Name: "basic",
    Topology: &types.Topology{
        Defaults: &types.NodeDefinition{Kind: "linux", Image: "alpine:3.20"},
        Nodes: map[string]*types.NodeDefinition{
            "client": {Exec: []string{"ip addr add 192.0.2.1/24 dev eth0"}},
            "server": {Exec: []string{"ip addr add 192.0.2.2/24 dev eth0"}},
        },
        Links: []*links.LinkDefinition{{
            Link: &links.LinkBriefRaw{Endpoints: []string{"client:eth0", "server:eth0"}},
        }},
    },
})
if err != nil { log.Fatal(err) }
lab, err := c.Start(ctx, &labcontainersv1.LabSpec{Topology: topology}, 30*time.Minute)
if err != nil { log.Fatal(err) }

result, err := lab.Node("client").Exec(ctx, "ping", "-c", "1", "192.0.2.2")
```

For fields not exposed by convenience methods, use the generated transport
directly: `c.RPC().Exec(ctx, &labcontainersv1.ExecRequest{Node: node.Ref(),
Argv: argv, Stdin: input, TimeoutMillis: 900000})`. Python exposes the same
surface as `client.rpc` and `node.ref`. These preserve the transport's request
types, responses, errors, and call options; no parallel options model is needed.
If an in-memory topology contains relative host bind paths, set
`topology.BaseDirectory` (Python `topology.base_directory`) to their absolute
base directory. This preserves their meaning when the daemon writes its private
Containerlab file; no caller-authored topology file is necessary.

`lab.Plan(ctx, proposedSource)` previews a full proposed topology through native
Containerlab dry-run and returns `*core.ApplyResult`, including
`RecreatedNodes`, `RestartedNodes`, and `NodeChangeReasons`. Pass `nil` to inspect
drift against the current topology. Python's `lab.plan(source(config))` returns
the same native JSON keys as a dictionary; `lab.plan()` inspects current drift.
The raw transport retains the unmodified native JSON, including new upstream
fields. Plans do not apply changes, reserve runtime state, or authorize a later
restart automatically. Drafts retain the session's isolation policy and existing
SDK-managed disk/bootstrap binds.

After inspecting/asserting the permitted impact, apply that exact native plan:
`lab.Apply(ctx, proposedSource, approvedPlan, nodeExtensions)` in Go or
`lab.apply(source(config), approved_plan, nodes=extensions)` in Python. The
server replans before deployment and rejects changed impact. The optional
extensions select serial `qga` control for newly added VMs; native Containerlab
objects still define their images, links, binds, environment, and lifecycle
behavior. Existing SDK-managed disk/bootstrap attachments are preserved, but
dynamic changes to those extensions are explicitly unsupported so far.

Topology application refuses foreign session containers and active faults,
checks isolation after convergence, and retains before/desired topologies and
the approved native plan as evidence. Native apply is not transactional: a
failure can leave partial changes. The session then reports `reconcile-failed`
and remains inspectable, retryable with a freshly reviewed plan, or destroyable.
These checks serialize operations within this daemon; they cannot reserve the
runtime against independent Docker/Containerlab changes by other processes.

Known native v0.79 limitation: interface ownership discovery excludes `eth0`
unconditionally, even if it is a declared data link on a node with no management
network. Native plans can therefore list an already-working `eth0` link in
`AddedLinks`. The SDK preserves that result; it does not claim it is a no-op or
automatically apply it. This upstream management-interface assumption still
needs correction before general live reconciliation can be qualified.

An isolated correction is included in `patches/containerlab-owned-eth0.patch`.
It uses native endpoint ownership to distinguish data `eth0` from an unmarked
management interface, in both discovery and namespace parking. Build it without
altering the host installation or module cache:

```sh
bash scripts/build-containerlab.sh /absolute/path/containerlab-owned-eth0
make build
LABCONTAINERS_CONTAINERLAB=/absolute/path/containerlab-owned-eth0 \
  LABCONTAINERS_LIVE=1 LABCONTAINERS_RECONCILE_LIVE=1 \
  go test ./pkg/client -run '^TestLive$' -count=1 -v
```

The builder verifies the pinned version, runs native link/core tests, and embeds
the patch digest in the CLI's commit metadata. `LABCONTAINERS_CONTAINERLAB`
selects an explicit CLI for the child daemon; ordinary launches still use the
host `containerlab`. The strict live no-op check fails on stock v0.79.0 and passes
with the correction. No management NIC is added, and no interface is renamed.
This is a narrow patched dependency, not a replacement topology or VM API;
upstreaming it remains outstanding. Go/Python live application tests qualify
container addition/removal; Go additionally verifies a new data link carries
traffic and unrelated containers retain both identity and start time. General
VM topology-change and storage-fixture qualification remain outstanding.

## Python

```python
from labcontainers import Client, api, containerlab as clab, source

topology = clab.Config(
    name="basic",
    topology=clab.Topology(
        defaults=clab.NodeConfig(kind="linux", image="alpine:3.20"),
        nodes={
            "client": clab.NodeConfig(exec=["ip addr add 192.0.2.1/24 dev eth0"]),
            "server": clab.NodeConfig(exec=["ip addr add 192.0.2.2/24 dev eth0"]),
        },
        links=[clab.LinkConfigShort(endpoints=["client:eth0", "server:eth0"])],
    ),
)

with Client() as client:
    lab = client.start(api.LabSpec(topology=source(topology)))
    result = lab.node("client").exec("ping", "-c", "1", "192.0.2.2")
    result.check_returncode()
```

Python bindings are generated from the exact schema version pinned in `go.mod`;
`make generate-containerlab` regenerates them reproducibly. Aliases such as
`network_mode` serialize to native `network-mode`. Unset schema defaults are not
sent. Arbitrary fields are retained where the upstream schema permits them, and
closed schema objects reject unknown fields.
Generated models are not a replacement for Containerlab's own validation;
kind-specific conditional constraints are still validated during deployment.

Python can be installed directly from a Git checkout (select the branch/commit
containing the bindings when testing unreleased changes):

```sh
python -m pip install 'git+https://github.com/AppMana/labcontainers.git@<commit>'
```

## Node.js

The npm transport remains supported; schema-generated topology objects in this
change are provided for Go and Python.

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

Evidence is retained by default under the OS user cache directory at
`labcontainers/artifacts/<session-id>` (on Linux, `$XDG_CACHE_HOME` or
`~/.cache`). `Session.Artifacts()` or the generated session's
`artifact_directory` field gives the actual path. Set `LabSpec.artifact_directory`
to choose a different location. Cleanup removes runtime state and attached test
disks, not this evidence. Retained files include the prepared native topology
and event log, including timeline failures; they are user-private because a
topology can contain credentials. They are not a complete guest log capture.
Remove retained evidence explicitly when it is no longer needed.

For now, keeping a session still requires an explicitly persistent state
directory; the default private client's temporary directory is removed on
close. Keep-on-failure lease handling is not yet complete.

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
make generate-containerlab
make test
make build
```

Containerlab is a separate BSD-3-Clause dependency. Labcontainers source is
Apache-2.0 licensed.
