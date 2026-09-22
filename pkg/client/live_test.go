package client

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/links"
	"github.com/srl-labs/containerlab/types"
)

// TestLive exercises the public Go SDK through a real labd, Containerlab, and
// Docker. It is opt-in because it needs privileged host networking.
func TestLive(t *testing.T) {
	if os.Getenv("LABCONTAINERS_LIVE") == "" {
		t.Skip("set LABCONTAINERS_LIVE=1 to run the privileged integration test")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, err := Launch(ctx, Options{LabdPath: filepath.Join(root, "bin", "labd")})
	if err != nil {
		t.Fatal(err)
	}
	var evidence string
	defer func() {
		if err := c.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
		if evidence != "" {
			for _, file := range []string{"events.jsonl", "topology.clab.yml"} {
				if _, err := os.Stat(filepath.Join(evidence, file)); err != nil {
					t.Errorf("evidence lost after client close: %v", err)
				}
			}
			t.Logf("retained evidence: %s", evidence)
		}
	}()
	prefix := "native-sdk"
	topology, err := clab.Source(&core.Config{Name: "basic", Prefix: &prefix, Topology: &types.Topology{
		Defaults: &types.NodeDefinition{Kind: "linux", Image: "alpine:3.20", NetworkMode: "none"},
		Nodes: map[string]*types.NodeDefinition{
			"client": {Exec: []string{"ip addr add 192.0.2.1/24 dev eth0"}},
			"server": {Exec: []string{"ip addr add 192.0.2.2/24 dev eth0"}},
		},
		Links: []*links.LinkDefinition{{Link: &links.LinkBriefRaw{Endpoints: []string{"client:eth0", "server:eth0"}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	lab, err := c.Start(ctx, &labv1.LabSpec{Topology: topology}, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	evidence = lab.Artifacts()
	plan, err := lab.Plan(ctx, topology)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.DryRun || plan.DeployedLab || len(plan.RecreatedNodes)+len(plan.RestartedNodes)+len(plan.AddedNodes)+len(plan.DeletedNodes) != 0 {
		t.Fatalf("unchanged native draft unexpectedly changes nodes: %+v", plan)
	}
	// Native v0.79 ownership discovery unconditionally excludes eth0, even
	// when it is an explicitly declared data interface with network-mode none.
	// Preserve this native diagnostic, rather than claiming a no-op plan or
	// filtering away a potentially disruptive link operation.
	if os.Getenv("LABCONTAINERS_RECONCILE_LIVE") != "" {
		if len(plan.AddedLinks)+len(plan.DeletedEndpoints) != 0 {
			t.Fatalf("unchanged declared data eth0 must be a native no-op: %+v", plan)
		}
	} else if len(plan.AddedLinks) != 1 || plan.AddedLinks[0] != "client:eth0 -- server:eth0" {
		t.Fatalf("native eth0 reconciliation behavior changed; requalify it: %+v", plan)
	}
	t.Logf("native unchanged-topology result: %+v", plan)
	draftConfig := &core.Config{Prefix: &prefix, Topology: &types.Topology{
		Defaults: &types.NodeDefinition{Kind: "linux", Image: "alpine:3.20", NetworkMode: "none"},
		Nodes: map[string]*types.NodeDefinition{
			"client": {Exec: []string{"ip addr add 192.0.2.1/24 dev eth0"}},
			"server": {Exec: []string{"ip addr add 192.0.2.2/24 dev eth0"}},
			"adhoc":  {Exec: []string{"ip addr add 192.0.3.2/24 dev eth1"}},
		},
		Links: []*links.LinkDefinition{
			{Link: &links.LinkBriefRaw{Endpoints: []string{"client:eth0", "server:eth0"}}},
			{Link: &links.LinkBriefRaw{Endpoints: []string{"client:eth1", "adhoc:eth1"}}},
		},
	}}
	draft, err := clab.Source(draftConfig)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = lab.Plan(ctx, draft)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.AddedNodes) != 1 || plan.AddedNodes[0] != "adhoc" || len(plan.RecreatedNodes)+len(plan.RestartedNodes)+len(plan.DeletedNodes) != 0 {
		t.Fatalf("native addition plan = %+v", plan)
	}
	draftConfig.Topology.Nodes["client"].Cmd = "sleep 100000"
	disruptive, err := clab.Source(draftConfig)
	if err != nil {
		t.Fatal(err)
	}
	impact, err := lab.Plan(ctx, disruptive)
	if err != nil {
		t.Fatal(err)
	}
	if len(impact.RecreatedNodes) != 1 || impact.RecreatedNodes[0] != "client" || impact.NodeChangeReasons["client"] == "" {
		t.Fatalf("native recreation impact missing: %+v", impact)
	}
	unchanged, err := lab.Plan(ctx, nil)
	if err != nil || len(unchanged.AddedNodes) != 0 {
		t.Fatalf("draft changed authoritative topology: result=%+v error=%v", unchanged, err)
	}
	if os.Getenv("LABCONTAINERS_RECONCILE_LIVE") != "" {
		identity := func() string {
			t.Helper()
			out, err := exec.CommandContext(ctx, "docker", "ps", "--no-trunc", "--filter", "label=labcontainers.appmana.com/session="+lab.ID(), "--filter", "label=clab-node-name=client", "--format", "{{.ID}}").Output()
			if err != nil || strings.TrimSpace(string(out)) == "" {
				t.Fatalf("client runtime identity: %s %v", out, err)
			}
			state, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Id}} {{.State.StartedAt}}", strings.TrimSpace(string(out))).Output()
			if err != nil {
				t.Fatal(err)
			}
			return string(state)
		}
		before := identity()
		if err := lab.Apply(ctx, draft, plan, nil); err != nil {
			t.Fatal(err)
		}
		if identity() != before {
			t.Fatal("adding an ad hoc node replaced an unrelated container")
		}
		added, err := lab.Node("adhoc").Exec(ctx, "true")
		if err != nil || added.GetExitCode() != 0 {
			t.Fatalf("added node not executable: %v %v", added, err)
		}
		configured, err := lab.Node("client").Exec(ctx, "ip", "addr", "add", "192.0.3.1/24", "dev", "eth1")
		if err != nil || configured.GetExitCode() != 0 {
			t.Fatalf("new link configuration failed: %v %v", configured, err)
		}
		probe, err := lab.Node("client").Exec(ctx, "ping", "-c", "1", "-W", "2", "192.0.3.2")
		if err != nil || probe.GetExitCode() != 0 {
			t.Fatalf("new declared link has no reachability: %v %v", probe, err)
		}
		remove, err := lab.Plan(ctx, topology)
		if err != nil {
			t.Fatal(err)
		}
		if len(remove.DeletedNodes) != 1 || remove.DeletedNodes[0] != "adhoc" || len(remove.RecreatedNodes)+len(remove.RestartedNodes) != 0 {
			t.Fatalf("unexpected removal impact: %+v", remove)
		}
		if err := lab.Apply(ctx, topology, remove, nil); err != nil {
			t.Fatal(err)
		}
		if identity() != before {
			t.Fatal("removing an ad hoc node replaced an unrelated container")
		}
		if _, err := lab.Node("adhoc").Exec(ctx, "true"); err == nil {
			t.Fatal("removed node still exposed")
		}
	}
	result, err := lab.Node("client").Exec(ctx, "ping", "-c", "1", "192.0.2.2")
	if err != nil {
		t.Fatal(err)
	}
	if result.GetExitCode() != 0 {
		t.Fatalf("ping exited %d: %s", result.GetExitCode(), result.GetStderr())
	}
	impairment, err := lab.Netem(ctx, "client", "eth0", &labv1.Netem{LossPercent: 100})
	if err != nil {
		t.Fatal(err)
	}
	result, err = lab.Node("client").Exec(ctx, "ping", "-c", "1", "-W", "1", "192.0.2.2")
	if err != nil || result.GetExitCode() == 0 {
		t.Fatalf("100%% packet loss did not cut reachability: result=%v error=%v", result, err)
	}
	if err := impairment.Revert(ctx); err != nil {
		t.Fatal(err)
	}
	fault, err := lab.SetLink(ctx, "client", "eth0", false)
	if err != nil {
		t.Fatal(err)
	}
	result, err = lab.Node("client").Exec(ctx, "ping", "-c", "1", "-W", "1", "192.0.2.2")
	if err != nil {
		t.Fatal(err)
	}
	if result.GetExitCode() == 0 {
		t.Fatal("reachability survived cutting the only declared path")
	}
	if err := fault.Revert(ctx); err != nil {
		t.Fatal(err)
	}
	result, err = lab.Node("client").Exec(ctx, "ping", "-c", "1", "-W", "2", "192.0.2.2")
	if err != nil {
		t.Fatal(err)
	}
	if result.GetExitCode() != 0 {
		t.Fatalf("reachability not restored: %s", result.GetStderr())
	}
}

// TestLiveVM proves the generic_vm/QGA boundary, attached-disk persistence,
// and abrupt power-cycle lifecycle used by storage-system crash tests. It is
// opt-in because it requires KVM and the labcontainers/vm-ubuntu:jammy image.
func TestLiveVM(t *testing.T) {
	if os.Getenv("LABCONTAINERS_VM_LIVE") == "" {
		t.Skip("set LABCONTAINERS_VM_LIVE=1 to run the KVM integration test")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	c, err := Launch(ctx, Options{LabdPath: filepath.Join(root, "bin", "labd")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()
	topology := vmTopology(t, "labcontainers/vm-ubuntu:jammy")
	lab, err := c.Start(ctx, &labv1.LabSpec{
		Topology: topology,
		Nodes: map[string]*labv1.NodeExtension{"vm": {
			Control: "qga",
			Disks:   []*labv1.Disk{{Name: "volume", SizeBytes: 1 << 30}},
		}},
	}, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	node := lab.Node("vm")
	waitExec := func(command string) *labv1.ExecResponse {
		t.Helper()
		var last error
		for deadline := time.Now().Add(3 * time.Minute); time.Now().Before(deadline); time.Sleep(2 * time.Second) {
			result, execErr := node.Exec(ctx, "sh", "-ec", command)
			if execErr == nil && result.GetExitCode() == 0 {
				return result
			}
			if execErr != nil {
				last = execErr
			} else {
				last = fmt.Errorf("exit %d: %s", result.GetExitCode(), result.GetStderr())
			}
		}
		t.Fatalf("guest command never succeeded: %v", last)
		return nil
	}
	waitExec("test -b /dev/disk/by-id/virtio-lc-volume")
	waitExec("mkfs.ext4 -F /dev/disk/by-id/virtio-lc-volume >/dev/null; mkdir -p /mnt/volume; mount /dev/disk/by-id/virtio-lc-volume /mnt/volume; printf durable >/mnt/volume/marker; sync")
	if err := node.PowerOff(ctx); err != nil {
		t.Fatal(err)
	}
	recoverVM(t, ctx, lab, "vm")
	result := waitExec("mkdir -p /mnt/volume; mount /dev/disk/by-id/virtio-lc-volume /mnt/volume; cat /mnt/volume/marker")
	if got := string(result.GetStdout()); got != "durable" {
		t.Fatalf("marker after abrupt power cycle = %q", got)
	}
}

// Both operating-system tests use the same native Containerlab objects. The
// generic SDK does not impose a Kubernetes fixture or its own VM node schema.
func vmTopology(t *testing.T, image string) *labv1.TopologySource {
	t.Helper()
	source, err := clab.Source(&core.Config{Name: "vm", Topology: &types.Topology{
		Defaults: &types.NodeDefinition{NetworkMode: "none"},
		Nodes: map[string]*types.NodeDefinition{
			"vm":   {Kind: "generic_vm", Image: image},
			"peer": {Kind: "linux", Image: "alpine:3.20"},
		},
		Links: []*links.LinkDefinition{{Link: &links.LinkBriefRaw{Endpoints: []string{"vm:eth1", "peer:eth1"}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return source
}
