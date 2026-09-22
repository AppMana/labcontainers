package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
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
	defer func() {
		if err := c.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()
	lab, err := c.Start(ctx, &labv1.LabSpec{Topology: &labv1.TopologySource{
		Source: &labv1.TopologySource_Path{Path: filepath.Join(root, "examples", "basic", "basic.clab.yml")},
	}}, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	result, err := lab.Node("client").Exec(ctx, "ping", "-c", "1", "192.0.2.2")
	if err != nil {
		t.Fatal(err)
	}
	if result.GetExitCode() != 0 {
		t.Fatalf("ping exited %d: %s", result.GetExitCode(), result.GetStderr())
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
	topology := []byte("name: ignored\ntopology:\n  nodes:\n    vm:\n      kind: generic_vm\n      image: labcontainers/vm-ubuntu:jammy\n      network-mode: none\n    peer:\n      kind: linux\n      image: alpine:3.20\n      network-mode: none\n  links:\n    - endpoints: [vm:eth1, peer:eth1]\n")
	lab, err := c.Start(ctx, &labv1.LabSpec{
		Topology: &labv1.TopologySource{Source: &labv1.TopologySource_Yaml{Yaml: topology}},
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
