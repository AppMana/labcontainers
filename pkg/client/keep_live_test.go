package client

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/types"
)

func TestLiveKeepSurvivesOwnerCloseAndExpires(t *testing.T) {
	if os.Getenv("LABCONTAINERS_LIVE") == "" {
		t.Skip("requires privileged Containerlab")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	c, err := Launch(ctx, Options{LabdPath: filepath.Join(root, "bin/labd")})
	if err != nil {
		t.Fatal(err)
	}
	source, err := clab.Source(&core.Config{Topology: &types.Topology{Nodes: map[string]*types.NodeDefinition{
		"kept": {Kind: "linux", Image: "alpine:3.20", NetworkMode: "none"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	lab, err := c.Start(ctx, &labv1.LabSpec{Topology: source}, time.Minute)
	if err != nil {
		_ = c.Close()
		t.Fatal(err)
	}
	// Exercise raw transport keep, not only the convenience method's local map.
	if _, err := c.RPC().KeepSession(ctx, &labv1.KeepSessionRequest{Id: lab.ID(), TtlSeconds: 10}); err != nil {
		_ = c.Close()
		t.Fatal(err)
	}
	socket, state := c.Socket(), c.StateDirectory()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lab.value.GetTopologyPath()); err != nil {
		t.Fatalf("kept topology erased: %v", err)
	}
	inspector, err := Dial(ctx, socket)
	if err != nil {
		t.Fatal(err)
	}
	defer inspector.Close()
	defer func() {
		// Failure cleanup addresses only this test's exact session.
		_, _ = inspector.RPC().DestroySession(context.Background(), &labv1.DestroySessionRequest{Id: lab.ID()})
	}()
	result, err := inspector.RPC().Exec(ctx, &labv1.ExecRequest{Node: lab.Node("kept").Ref(), Argv: []string{"true"}})
	if err != nil || result.GetExitCode() != 0 {
		t.Fatalf("kept lab not usable: %v %v", result, err)
	}
	for {
		_, err := os.Stat(filepath.Join(state, "sessions", lab.ID()))
		if os.IsNotExist(err) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("kept lease was not reaped")
		case <-time.After(100 * time.Millisecond):
		}
	}
	out, err := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", "label=labcontainers.appmana.com/session="+lab.ID()).Output()
	if err != nil || strings.TrimSpace(string(out)) != "" {
		t.Fatalf("expired lab still exists: %s %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(lab.Artifacts(), "events.jsonl")); err != nil {
		t.Fatalf("expiry erased evidence: %v", err)
	}
}
