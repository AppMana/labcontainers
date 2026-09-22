package client

import (
	"context"
	"encoding/base64"
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

	// Configuration is part of native node startup, including a fresh guest
	// when vrnetlab reconciliation recreates the wrapper. It uses serial QGA,
	// not a management connection or a caller-side repair after the crash.
	network := base64.StdEncoding.EncodeToString([]byte("[Match]\nName=en*\n[Network]\nAddress=192.0.2.1/24\nDHCP=no\nLinkLocalAddressing=no\nIPv6AcceptRA=no\n"))
	configureNetwork := "cloud-init status --wait >/dev/null; printf %s " + network + " | base64 -d > /etc/systemd/network/00-labcontainers.network; systemctl restart systemd-networkd"
	// QGA may not be listening when Containerlab's exec stage first runs.
	startup := fmt.Sprintf("i=0; until /labcontainers-guest exec 10s 0 sh -ec %q; do i=$((i+1)); [ \"$i\" -lt 18 ] || exit 1; sleep 2; done", configureNetwork)
	topology, err := clab.Source(&core.Config{Name: "bridge-restart", Topology: &types.Topology{
		Defaults: &types.NodeDefinition{NetworkMode: "none"},
		Nodes: map[string]*types.NodeDefinition{
			"vm":     {Kind: "generic_vm", Image: vmImage, Exec: []string{fmt.Sprintf("sh -ec %q", startup)}},
			"switch": {Kind: "linux", Image: "alpine:3.20"},
			"peer":   {Kind: "linux", Image: "alpine:3.20"},
		},
		Links: []*links.LinkDefinition{
			{Link: &links.LinkBriefRaw{Endpoints: []string{"vm:eth1", "switch:eth1"}}},
			{Link: &links.LinkBriefRaw{Endpoints: []string{"peer:eth1", "switch:eth2"}}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	lab, err := c.Start(ctx, &labv1.LabSpec{
		Topology: topology,
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
	execOK("vm", "sh", "-ec", `set -- /sys/class/net/*; test "$#" -eq 2; test -z "$(ip -4 route show default)"; test -z "$(ip -6 route show default)"`)
	waitPing("before crash")
	fault, err := lab.SetLink(ctx, "switch", "eth1", false)
	if err != nil {
		t.Fatal(err)
	}
	probe, err := lab.Node("peer").Exec(ctx, "ping", "-c", "1", "-W", "1", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if probe.GetExitCode() == 0 {
		t.Fatal("VM remained reachable after its only declared path was cut")
	}
	execOK("vm", "true") // Serial QGA control survives the dataplane cut.
	if err := fault.Revert(ctx); err != nil {
		t.Fatal(err)
	}
	waitPing("after restoring declared path")
	t.Log("one guest NIC, no default routes; cutting the declared path breaks reachability but preserves QGA")
	if err := vm.Crash(ctx); err != nil {
		t.Fatal(err)
	}
	if err := vm.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitVM()
	waitPing("after crash and start without caller-side network repair")
}
