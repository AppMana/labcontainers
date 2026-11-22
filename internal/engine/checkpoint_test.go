package engine

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

type checkpointRunner struct {
	running string
	argv    []string
}

func (r *checkpointRunner) Run(_ context.Context, _ io.Reader, argv ...string) (Result, error) {
	line := strings.Join(argv, " ")
	r.argv = append(r.argv, line)
	switch {
	case strings.HasPrefix(line, "docker ps --all"):
		return Result{Stdout: []byte("abc123\n")}, nil
	case strings.HasPrefix(line, "docker inspect"):
		return Result{Stdout: []byte(r.running + " sha256:base\n")}, nil
	case strings.HasPrefix(line, "docker diff"):
		return Result{Stdout: []byte("C /run\nA /run/labcontainers-qga.sock\nA /base-overlay.qcow2\nA /tmp/nested-overlay.qcow2\n")}, nil
	}
	return Result{}, nil
}

func TestNodeDisksMoveOnlyWhileStoppedAndOnTheSameImage(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	r := &checkpointRunner{running: "true"}
	c := &Containerlab{Runner: r}
	if _, _, err := c.ExportNode(ctx, "/topo.yml", "vm", dir); err == nil {
		t.Fatal("exported a running node")
	}
	r.running = "false"
	image, files, err := c.ExportNode(ctx, "/topo.yml", "vm", dir)
	if err != nil || image != "sha256:base" || len(files) != 1 || files[0] != "base-overlay.qcow2" {
		t.Fatalf("export: %s %v %v", image, files, err)
	}
	if last := r.argv[len(r.argv)-1]; last != "docker cp abc123:/base-overlay.qcow2 "+filepath.Join(dir, "base-overlay.qcow2") {
		t.Fatalf("unexpected copy: %s", last)
	}
	if err := c.ImportNode(ctx, "/topo.yml", "vm", dir, "sha256:other", files); err == nil {
		t.Fatal("restored an overlay onto a different base image")
	}
	for _, bad := range []string{"../escape-overlay.qcow2", "config.json"} {
		if err := c.ImportNode(ctx, "/topo.yml", "vm", dir, "sha256:base", []string{bad}); err == nil {
			t.Fatalf("accepted checkpoint file %q", bad)
		}
	}
	if err := c.ImportNode(ctx, "/topo.yml", "vm", dir, "sha256:base", files); err != nil {
		t.Fatal(err)
	}
	if last := r.argv[len(r.argv)-1]; last != "docker cp "+filepath.Join(dir, "base-overlay.qcow2")+" abc123:/base-overlay.qcow2" {
		t.Fatalf("unexpected copy: %s", last)
	}
}
