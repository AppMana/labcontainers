package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	if len(plan.AddedLinks) != 1 || plan.AddedLinks[0] != "client:eth0 -- server:eth0" {
		t.Fatalf("native eth0 reconciliation behavior changed; requalify it: %+v", plan)
	}
	t.Logf("upstream eth0 discovery limitation remains visible: %+v", plan)
	draftConfig := &core.Config{Prefix: &prefix, Topology: &types.Topology{
		Defaults: &types.NodeDefinition{Kind: "linux", Image: "alpine:3.20", NetworkMode: "none"},
		Nodes: map[string]*types.NodeDefinition{
			"client": {Exec: []string{"ip addr add 192.0.2.1/24 dev eth0"}},
			"server": {Exec: []string{"ip addr add 192.0.2.2/24 dev eth0"}},
			"adhoc":  {},
		},
		Links: []*links.LinkDefinition{{Link: &links.LinkBriefRaw{Endpoints: []string{"client:eth0", "server:eth0"}}}},
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
	if err := node.Start(ctx); err != nil {
		t.Fatal(err)
	}
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
