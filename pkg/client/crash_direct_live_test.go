package client

import (
	"context"
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

// A small real-netns reproduction of the missing eth1 after Windows VM crash.
// No Apply/redeploy or caller-side link repair may mask a failed native Start.
func TestLiveCrashStartPreservesDirectLink(t *testing.T) {
	if os.Getenv("LABCONTAINERS_CRASH_LIVE") != "1" {
		t.Skip("explicit real-netns crash test required")
	}
	daemon, state := os.Getenv("LABCONTAINERS_LABD"), os.Getenv("LABCONTAINERS_STATE_DIR")
	if !filepath.IsAbs(daemon) || !filepath.IsAbs(state) || filepath.Clean(state) == "/" {
		t.Fatal("explicit daemon and persistent state paths required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, err := Launch(ctx, Options{LabdPath: daemon, StateDir: state})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	topology, err := clab.Source(&core.Config{Topology: &types.Topology{
		Defaults: &types.NodeDefinition{Kind: "linux", Image: "alpine:3.20", ImagePullPolicy: "Never", NetworkMode: "none"},
		Nodes: map[string]*types.NodeDefinition{
			"client": {Exec: []string{"ip addr add 192.0.2.1/24 dev eth1"}},
			"peer":   {Exec: []string{"ip addr add 192.0.2.2/24 dev eth1"}},
		},
		Links: []*links.LinkDefinition{{Link: &links.LinkBriefRaw{Endpoints: []string{"client:eth1", "peer:eth1"}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	lab, err := c.Start(ctx, &labv1.LabSpec{Topology: topology}, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := lab.Keep(ctx, time.Hour); err != nil {
		t.Fatal(err)
	}
	t.Logf("retained session=%s socket=%s evidence=%s", lab.ID(), c.Socket(), lab.Artifacts())
	read := func(node string, argv ...string) string {
		t.Helper()
		r, err := lab.Node(node).Exec(ctx, argv...)
		if err != nil {
			t.Fatal(err)
		}
		if r.ExitCode != 0 {
			t.Fatalf("%s exit %d: %s %s", node, r.ExitCode, r.Stdout, r.Stderr)
		}
		return string(r.Stdout)
	}
	before := read("peer", "cat", "/sys/class/net/eth1/ifindex", "/sys/class/net/eth1/address")
	read("client", "sh", "-ec", "printf original > /survives-crash; sync")
	read("peer", "ping", "-c", "1", "-W", "2", "192.0.2.1")
	if err := lab.Node("client").Crash(ctx); err != nil {
		t.Fatal(err)
	}
	if err := lab.Node("client").Start(ctx); err != nil {
		t.Fatal(err)
	}
	if after := read("peer", "cat", "/sys/class/net/eth1/ifindex", "/sys/class/net/eth1/address"); after != before {
		t.Fatalf("peer link replaced: %q -> %q", before, after)
	}
	if read("client", "cat", "/survives-crash") != "original" {
		t.Fatal("container writable state lost")
	}
	read("peer", "ping", "-c", "1", "-W", "2", "192.0.2.1")
	t.Log("DIRECT_LINK_CRASH_START_COMPLETE")
}
