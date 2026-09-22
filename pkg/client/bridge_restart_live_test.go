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

// TestLiveVMCrashRestoresRuntimeBridgeMembership proves that restarting a VM
// restores the topology link's runtime bridge membership. The caller builds
// the Alpine bridge once; no network repair is performed after Crash/Start.
func TestLiveVMCrashRestoresRuntimeBridgeMembership(t *testing.T) {
	if os.Getenv("LABCONTAINERS_VM_LIVE") == "" {
		t.Skip("set LABCONTAINERS_VM_LIVE=1 to run the KVM integration test")
	}

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	networkDir := t.TempDir()
	if err := os.Chmod(networkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	networkPath := filepath.Join(networkDir, "vm.yaml")
	network := []byte("version: 2\nethernets:\n  topology:\n    match:\n      name: 'en*'\n    addresses: [192.0.2.1/24]\n    optional: true\n")
	if err := os.WriteFile(networkPath, network, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	labdPath := os.Getenv("LABCONTAINERS_LABD")
	if labdPath == "" {
		labdPath = filepath.Join(root, "bin", "labd")
	}
	vmImage := os.Getenv("LABCONTAINERS_VM_IMAGE")
	if vmImage == "" {
		vmImage = "labcontainers/vm-ubuntu:jammy"
	}
	c, err := Launch(ctx, Options{LabdPath: labdPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()

	topology := []byte(fmt.Sprintf(`name: ignored
topology:
  nodes:
    vm:
      kind: generic_vm
      image: %q
      network-mode: none
      binds:
        - %q
    switch:
      kind: linux
      image: alpine:3.20
      network-mode: none
    peer:
      kind: linux
      image: alpine:3.20
      network-mode: none
  links:
    - endpoints: [vm:eth1, switch:eth1]
    - endpoints: [peer:eth1, switch:eth2]
`, vmImage, networkPath+":/extra-network.yaml:ro"))
	lab, err := c.Start(ctx, &labv1.LabSpec{
		Topology: &labv1.TopologySource{Source: &labv1.TopologySource_Yaml{Yaml: topology}},
		Nodes:    map[string]*labv1.NodeExtension{"vm": {Control: "qga"}},
	}, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	execOK := func(node string, argv ...string) {
		t.Helper()
		result, err := lab.Node(node).Exec(ctx, argv...)
		if err != nil {
			t.Fatal(err)
		}
		if result.GetExitCode() != 0 {
			t.Fatalf("%s command exited %d: %s%s", node, result.GetExitCode(), result.GetStdout(), result.GetStderr())
		}
	}
	execOK("switch", "sh", "-ec", "ip link add br0 type bridge; for i in eth1 eth2; do ip link set $i master br0; ip link set $i up; done; ip link set br0 up")
	execOK("peer", "sh", "-ec", "ip address add 192.0.2.2/24 dev eth1; ip link set eth1 up")

	vm := lab.Node("vm")
	waitVM := func() {
		t.Helper()
		var last error
		for deadline := time.Now().Add(3 * time.Minute); time.Now().Before(deadline); time.Sleep(2 * time.Second) {
			result, execErr := vm.Exec(ctx, "sh", "-ec", "cloud-init status --wait >/dev/null; ip -4 address show | grep -q '192.0.2.1/24'")
			if execErr == nil && result.GetExitCode() == 0 {
				return
			}
			if execErr != nil {
				last = execErr
			} else {
				last = fmt.Errorf("exit %d: %s%s", result.GetExitCode(), result.GetStdout(), result.GetStderr())
			}
		}
		t.Fatalf("VM network never became ready: %v", last)
	}
	waitPing := func(stage string) {
		t.Helper()
		var last error
		for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); time.Sleep(time.Second) {
			result, execErr := lab.Node("peer").Exec(ctx, "ping", "-c", "1", "-W", "1", "192.0.2.1")
			if execErr == nil && result.GetExitCode() == 0 {
				return
			}
			if execErr != nil {
				last = execErr
			} else {
				last = fmt.Errorf("exit %d: %s%s", result.GetExitCode(), result.GetStdout(), result.GetStderr())
			}
		}
		t.Fatalf("peer could not reach VM %s: %v", stage, last)
	}

	waitVM()
	waitPing("before crash")
	if err := vm.Crash(ctx); err != nil {
		t.Fatal(err)
	}
	if err := vm.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitVM()
	waitPing("after crash and start without caller-side network repair")
}
