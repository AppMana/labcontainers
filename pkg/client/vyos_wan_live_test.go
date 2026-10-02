package client_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/pkg/client"
	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/appmana/labcontainers/pkg/network"
	"github.com/appmana/labcontainers/pkg/network/vyos"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/links"
	"github.com/srl-labs/containerlab/types"
)

// This small VM gate qualifies the router and fault mechanism before an
// expensive Kubernetes installation. WAN is an explicitly declared upstream
// subnet, not Internet access. BGP/Calico are downstream qualification gates.
func TestLiveVyOSRoutedWAN(t *testing.T) {
	image, peer := os.Getenv("LABCONTAINERS_VYOS_IMAGE"), os.Getenv("LABCONTAINERS_VYOS_PEER_IMAGE")
	if image == "" || peer == "" {
		t.Skip("explicit VyOS VM and Linux peer images required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	c, err := client.Launch(ctx, client.Options{LabdPath: os.Getenv("LABCONTAINERS_LABD"), StateDir: os.Getenv("LABCONTAINERS_STATE_DIR")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()
	nodes := map[string]*types.NodeDefinition{
		"tor": {Kind: "generic_vm", Image: image, ImagePullPolicy: "Never", NetworkMode: "none"},
	}
	for _, name := range []string{"left", "right", "wan"} {
		nodes[name] = &types.NodeDefinition{Kind: "linux", Image: peer, ImagePullPolicy: "Never", NetworkMode: "none", Entrypoint: "/bin/sleep", Cmd: "infinity"}
	}
	topology, err := clab.Source(&core.Config{Topology: &types.Topology{Nodes: nodes, Links: []*links.LinkDefinition{
		{Link: &links.LinkVEthRaw{Endpoints: []*links.EndpointRaw{{Node: "left", Iface: "eth1"}, {Node: "tor", Iface: "eth1", MAC: "02:00:00:00:00:01"}}}},
		{Link: &links.LinkVEthRaw{Endpoints: []*links.EndpointRaw{{Node: "right", Iface: "eth1"}, {Node: "tor", Iface: "eth2", MAC: "02:00:00:00:00:02"}}}},
		{Link: &links.LinkVEthRaw{Endpoints: []*links.EndpointRaw{{Node: "tor", Iface: "eth3", MAC: "02:00:00:00:00:03"}, {Node: "wan", Iface: "eth1"}}}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	lab, err := c.Start(ctx, &labv1.LabSpec{Topology: topology, Nodes: map[string]*labv1.NodeExtension{"tor": {Control: "qga"}}}, 8*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("session=%s artifacts=%s", lab.ID(), lab.Artifacts())
	defer func() {
		if !t.Failed() {
			return
		}
		dctx, done := context.WithTimeout(context.Background(), 20*time.Second)
		defer done()
		for _, name := range []string{"tor", "left", "right", "wan"} {
			out, err := lab.Node(name).Commands().Exec(dctx, "sh", "-c", "ip -d link; ip addr; ip route; ip -6 route")
			t.Logf("%s diagnostics: %s (%v)", name, out, err)
		}
	}()
	_, err = lab.RunTimeline(ctx, &labv1.TimelineAction{Action: &labv1.TimelineAction_WaitExec{WaitExec: &labv1.WaitExec{
		Exec:          &labv1.ExecRequest{Node: &labv1.NodeRef{Node: "tor"}, Argv: []string{"cat", "/etc/os-release"}, TimeoutMillis: 5000},
		TimeoutMillis: 300000, RetryMillis: 2000, StdoutContains: []byte("ID=vyos"),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	run := func(node string, args ...string) string {
		t.Helper()
		out, err := lab.Node(node).Commands().Exec(ctx, args...)
		if err != nil {
			t.Fatalf("%s: %s: %v", node, out, err)
		}
		return string(out)
	}
	physical := run("tor", "sh", "-ec", `for p in /sys/class/net/*; do test ! -e "$p/device" || basename "$p"; done`)
	if len(strings.Fields(physical)) != 3 {
		t.Fatalf("unexpected physical NICs: %q", physical)
	}
	ports, err := network.InterfaceNames(ctx, lab.Node("tor").Commands(), "02:00:00:00:00:01", "02:00:00:00:00:02", "02:00:00:00:00:03")
	if err != nil {
		t.Fatal(err)
	}
	set := func(path ...string) vyos.Command { return vyos.Command{Operation: "set", Path: path} }
	commands := []vyos.Command{
		set("interfaces", "ethernet", ports[0], "description", "left LAN"),
		set("interfaces", "ethernet", ports[1], "description", "right LAN"),
		set("interfaces", "bridge", "br0", "member", "interface", ports[0]),
		set("interfaces", "bridge", "br0", "member", "interface", ports[1]),
		set("interfaces", "bridge", "br0", "address", "192.0.2.1/24"),
		set("interfaces", "bridge", "br0", "address", "fd00:10::1/64"),
		set("interfaces", "ethernet", ports[2], "address", "198.18.0.2/30"),
		set("interfaces", "ethernet", ports[2], "address", "2001:db8:ffff::2/64"),
	}
	if err := vyos.Apply(ctx, lab.Node("tor").Commands(), commands); err != nil {
		t.Fatal(err)
	}
	for name, suffix := range map[string]int{"left": 10, "right": 20} {
		run(name, "sh", "-ec", fmt.Sprintf(`ip link set eth1 up; ip addr add 192.0.2.%d/24 dev eth1; ip -6 addr add fd00:10::%d/64 dev eth1; ip route add default via 192.0.2.1; ip -6 route add default via fd00:10::1`, suffix, suffix))
	}
	run("wan", "sh", "-ec", `ip link set eth1 up; ip addr add 198.18.0.1/30 dev eth1; ip -6 addr add 2001:db8:ffff::1/64 dev eth1; ip route add default via 198.18.0.2; ip -6 route add default via 2001:db8:ffff::2`)
	probe := func(node, address string, up bool) {
		t.Helper()
		r, err := lab.Node(node).ExecWithTimeout(ctx, 10*time.Second, "ping", "-c", "2", "-W", "2", address)
		if err != nil {
			t.Fatal("control transport failure is not a network outage:", err)
		}
		if (up && r.ExitCode != 0) || (!up && r.ExitCode != 1) {
			t.Fatalf("%s -> %s up=%v exit=%d: %s %s", node, address, up, r.ExitCode, r.Stdout, r.Stderr)
		}
	}
	// Wait for IPv6 DAD, without changing addresses after a failed assertion.
	for _, name := range []string{"tor", "left", "right", "wan"} {
		_, err := lab.RunTimeline(ctx, &labv1.TimelineAction{Action: &labv1.TimelineAction_WaitExec{WaitExec: &labv1.WaitExec{
			Exec: &labv1.ExecRequest{Node: &labv1.NodeRef{Node: name}, Argv: []string{"sh", "-ec", `test -z "$(ip -6 addr show tentative)"`}, TimeoutMillis: 5000}, TimeoutMillis: 15000, RetryMillis: 500,
		}}})
		if err != nil {
			t.Fatalf("%s IPv6 address readiness: %v", name, err)
		}
	}
	for _, name := range []string{"left", "right"} {
		probe(name, "198.18.0.1", true)
		probe(name, "2001:db8:ffff::1", true)
	}
	probe("wan", "192.0.2.10", true)
	probe("wan", "fd00:10::20", true)
	fault, err := lab.SetLink(ctx, "tor", "eth3", false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		rctx, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		if err := fault.Revert(rctx); err != nil {
			t.Error(err)
		}
	}()
	probe("left", "198.18.0.1", false)
	probe("right", "2001:db8:ffff::1", false)
	probe("left", "192.0.2.20", true)
	probe("right", "fd00:10::10", true)
	if err := fault.Revert(ctx); err != nil {
		t.Fatal(err)
	}
	probe("left", "198.18.0.1", true)
	probe("right", "2001:db8:ffff::1", true)
	// Saved routing must survive a real boot without reapplying configuration.
	// QGA starts before VyOS finishes udev renaming and its native config load.
	boot := run("tor", "cat", "/proc/sys/kernel/random/boot_id")
	if err := lab.Node("tor").Restart(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		attempt, done := context.WithTimeout(ctx, 5*time.Second)
		current, bootErr := lab.Node("tor").Commands().Exec(attempt, "cat", "/proc/sys/kernel/random/boot_id")
		done()
		if bootErr == nil && strings.TrimSpace(string(current)) != "" && string(current) != boot {
			attempt, done = context.WithTimeout(ctx, 5*time.Second)
			state, readyErr := lab.Node("tor").Commands().Exec(attempt, "systemctl", "show", "vyos-router.service", "-p", "SubState", "-p", "Result")
			done()
			if readyErr == nil && strings.Contains(string(state), "SubState=exited") && strings.Contains(string(state), "Result=success") {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("VyOS did not complete native configuration loading on a new boot")
		}
		time.Sleep(time.Second)
	}
	after, err := network.InterfaceNames(ctx, lab.Node("tor").Commands(), "02:00:00:00:00:01", "02:00:00:00:00:02", "02:00:00:00:00:03")
	if err != nil || !reflect.DeepEqual(ports, after) {
		t.Fatalf("saved router port identity changed: %v -> %v: %v", ports, after, err)
	}
	for _, name := range []string{"left", "right"} {
		probe(name, "198.18.0.1", true)
		probe(name, "2001:db8:ffff::1", true)
	}
	probe("wan", "192.0.2.10", true)
	probe("wan", "fd00:10::20", true)
	if output := run("tor", "ip", "-o", "route", "show", "default"); strings.TrimSpace(output) != "" {
		t.Fatalf("undeclared Internet route: %s", output)
	}
	t.Log("VYOS_ROUTED_WAN_RECOVERY_COMPLETE")
}
