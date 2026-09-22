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

func TestLivePersistentReconnectAndDestroy(t *testing.T) {
	if os.Getenv("LABCONTAINERS_LIVE") != "1" {
		t.Skip("requires privileged Containerlab")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	opts := PersistentOptions{StateDir: filepath.Join(dir, "state"), RefPath: filepath.Join(dir, "session.json"), LabdPath: filepath.Join(root, "bin/labd"), TTL: time.Minute}
	source, err := clab.Source(&core.Config{Topology: &types.Topology{Nodes: map[string]*types.NodeDefinition{
		"guest": {Kind: "linux", Image: "alpine:3.20", ImagePullPolicy: "Never", NetworkMode: "none"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	value, err := DeployPersistent(ctx, opts, &labv1.LabSpec{Topology: source})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = DestroyPersistent(cleanup, opts)
	}()
	// Both subsequent operations must reconnect; a launch would fail here.
	opts.LabdPath = "/nonexistent/must-not-launch"
	c, session, err := OpenPersistent(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	socket := c.Socket()
	result, runErr := session.Node("guest").Exec(ctx, "true")
	closeErr := c.Close()
	if runErr != nil || result.GetExitCode() != 0 || closeErr != nil {
		t.Fatalf("reconnected command: %v %v %v", result, runErr, closeErr)
	}
	if err := DestroyPersistent(ctx, opts); err != nil {
		t.Fatal(err)
	}
	containers, err := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", "label=labcontainers.appmana.com/session="+value.Id).Output()
	if err != nil || strings.TrimSpace(string(containers)) != "" {
		t.Fatalf("owned runtime remains: %s %v", containers, err)
	}
	if _, err := os.Stat(opts.RefPath); !os.IsNotExist(err) {
		t.Fatalf("reference remains: %v", err)
	}
	// The detached daemon exits after observing its empty store. Do not let
	// TempDir cleanup remove that store before its final ownership check.
	for {
		if _, err := os.Stat(socket); os.IsNotExist(err) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("daemon did not exit after teardown", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Logf("retained evidence: %s", value.ArtifactDirectory)
}
