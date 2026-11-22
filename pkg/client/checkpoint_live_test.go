package client

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/types"
)

// A real VM's root overlay and data disk come back from a checkpoint, both
// in the session that made it and in a later session built from the same
// topology. Opt-in: needs KVM and a prepared Ubuntu wrapper image.
func TestLiveCheckpointRestoresRootAndDataDisks(t *testing.T) {
	if os.Getenv("LABCONTAINERS_CHECKPOINT_LIVE") != "1" {
		t.Skip("explicit real-VM checkpoint test required")
	}
	daemon, state, image := os.Getenv("LABCONTAINERS_LABD"), os.Getenv("LABCONTAINERS_STATE_DIR"), os.Getenv("LABCONTAINERS_VM_IMAGE")
	if !filepath.IsAbs(daemon) || !filepath.IsAbs(state) || filepath.Clean(state) == "/" || image == "" {
		t.Fatal("explicit daemon, persistent state directory and VM image required")
	}
	checkpointDir := filepath.Join(state, "checkpoint")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	// A daemon exits with its last session, so the later session has its own.
	start := func(name string) *Session {
		t.Helper()
		c, err := Launch(ctx, Options{LabdPath: daemon, StateDir: filepath.Join(state, name)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		topology, err := clab.Source(&core.Config{Topology: &types.Topology{Nodes: map[string]*types.NodeDefinition{
			"vm": {Kind: "generic_vm", Image: image, ImagePullPolicy: "Never", NetworkMode: "none"},
		}}})
		if err != nil {
			t.Fatal(err)
		}
		lab, err := c.Start(ctx, &labv1.LabSpec{Topology: topology, Nodes: map[string]*labv1.NodeExtension{
			"vm": {Control: "qga", Disks: []*labv1.Disk{{Name: "data", SizeBytes: 1 << 30}}},
		}}, 10*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		return lab
	}
	sh := func(lab *Session, script string) string {
		t.Helper()
		deadline := time.Now().Add(4 * time.Minute)
		for {
			r, err := lab.Node("vm").ExecWithTimeout(ctx, 30*time.Second, "sh", "-ec", script)
			if err == nil && r.ExitCode == 0 {
				return strings.TrimSpace(string(r.Stdout))
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: %v %+v", script, err, r)
			}
			time.Sleep(2 * time.Second)
		}
	}
	cycle := func(lab *Session, between func()) {
		t.Helper()
		if err := lab.Node("vm").PowerOff(ctx); err != nil {
			t.Fatal(err)
		}
		between()
		if err := lab.Node("vm").Start(ctx); err != nil {
			t.Fatal(err)
		}
	}
	const disk = "/dev/disk/by-id/virtio-lc-data"
	const read = "mkdir -p /mnt/data; mountpoint -q /mnt/data || mount " + disk + " /mnt/data; cat /root/marker /mnt/data/marker"

	lab := start("first")
	lease, err := lab.HoldLease(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	sh(lab, "test -b "+disk+"; mkfs.ext4 -q "+disk+"; mkdir -p /mnt/data; mount "+disk+" /mnt/data; echo provisioned > /root/marker; echo provisioned > /mnt/data/marker; sync")
	if _, err := lab.Checkpoint(ctx, checkpointDir, nil); err == nil {
		t.Fatal("checkpointed a running VM")
	}
	cycle(lab, func() {
		saved, err := lab.Checkpoint(ctx, checkpointDir, map[string]string{"state": "provisioned"})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("checkpoint: %d bytes, nodes %v", saved.GetSizeBytes(), saved.GetNodes())
	})
	sh(lab, "mkdir -p /mnt/data; mount "+disk+" /mnt/data; echo changed > /root/marker; echo changed > /mnt/data/marker; sync")
	cycle(lab, func() {
		if _, err := lab.Restore(ctx, checkpointDir); err != nil {
			t.Fatal(err)
		}
	})
	if got := sh(lab, read); got != "provisioned\nprovisioned" {
		t.Fatalf("same session after restore: %q", got)
	}
	if err := lab.Destroy(ctx); err != nil {
		t.Fatal(err)
	}
	manifest, err := ReadCheckpoint(checkpointDir)
	if err != nil || manifest.Labels["state"] != "provisioned" {
		t.Fatalf("the checkpoint did not outlive its session: %+v %v", manifest, err)
	}

	later := start("later")
	cycle(later, func() {
		if _, err := later.Restore(ctx, checkpointDir); err != nil {
			t.Fatal(err)
		}
	})
	if got := sh(later, read); got != "provisioned\nprovisioned" {
		t.Fatalf("later session after restore: %q", got)
	}
	if lease.Err() != nil {
		t.Fatalf("lease renewal: %v", lease.Err())
	}
}
