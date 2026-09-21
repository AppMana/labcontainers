package client

import (
	"context"
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
