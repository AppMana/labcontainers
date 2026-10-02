package client_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/pkg/client"
	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/appmana/labcontainers/pkg/network/vyos"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/types"
)

func TestLiveVyOSSerialControl(t *testing.T) {
	image := os.Getenv("LABCONTAINERS_VYOS_IMAGE")
	if image == "" {
		t.Skip("explicit installed VyOS image and KVM required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
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
	source, err := clab.Source(&core.Config{Topology: &types.Topology{Nodes: map[string]*types.NodeDefinition{
		"vyos": {Kind: "generic_vm", Image: image, ImagePullPolicy: "Never", NetworkMode: "none"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	lab, err := c.Start(ctx, &labv1.LabSpec{Topology: source, Nodes: map[string]*labv1.NodeExtension{"vyos": {Control: "qga"}}}, 7*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("session=%s artifacts=%s", lab.ID(), lab.Artifacts())
	_, err = lab.RunTimeline(ctx, &labv1.TimelineAction{Action: &labv1.TimelineAction_WaitExec{WaitExec: &labv1.WaitExec{
		Exec:          &labv1.ExecRequest{Node: &labv1.NodeRef{Node: "vyos"}, Argv: []string{"cat", "/etc/os-release"}, TimeoutMillis: 5000},
		TimeoutMillis: 300000, RetryMillis: 2000, StdoutContains: []byte("ID=vyos"),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.Node("vyos").ExecWithTimeout(ctx, 10*time.Second, "sh", "-ec", `cat /usr/share/vyos/version.json; ip -o link; for p in /sys/class/net/*; do test ! -e "$p/device" || exit 1; done; ip route show default; ip -6 route show default`)
	if err != nil || r.ExitCode != 0 {
		t.Fatalf("native VyOS/control isolation: %v %+v", err, r)
	}
	if strings.Contains(string(r.Stdout), "default via") {
		t.Fatal("undeclared default route")
	}
	t.Log(string(r.Stdout))
	if err := vyos.Apply(ctx, lab.Node("vyos").Commands(), []vyos.Command{{Operation: "set", Path: []string{"system", "host-name", "lab-tor"}}}); err != nil {
		t.Fatal(err)
	}
	if err := lab.Node("vyos").Restart(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = lab.RunTimeline(ctx, &labv1.TimelineAction{Action: &labv1.TimelineAction_WaitExec{WaitExec: &labv1.WaitExec{
		Exec:          &labv1.ExecRequest{Node: &labv1.NodeRef{Node: "vyos"}, Argv: []string{"hostname"}, TimeoutMillis: 5000},
		TimeoutMillis: 180000, RetryMillis: 2000, StdoutContains: []byte("lab-tor"),
	}}})
	if err != nil {
		t.Fatal("native committed configuration did not survive restart:", err)
	}
}
