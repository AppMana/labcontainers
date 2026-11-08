package client

import (
	"bufio"
	"context"
	"fmt"
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

func TestLivePersistentReaperTearsDownWhenOwnerKilled(t *testing.T) {
	if os.Getenv("LABCONTAINERS_PERSISTENT_REAPER_CHILD") == "1" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		root, err := filepath.Abs("../..")
		if err != nil {
			t.Fatal(err)
		}
		dir := os.Getenv("LABCONTAINERS_PERSISTENT_REAPER_DIR")
		opts := PersistentOptions{
			StateDir: filepath.Join(dir, "state"),
			RefPath:  filepath.Join(dir, "session.json"),
			LabdPath: filepath.Join(root, "bin/labd"),
			TTL:      10 * time.Minute,
		}
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
		c, _, err := OpenPersistent(ctx, opts)
		if err != nil {
			t.Fatal(err)
		}
		socket := c.Socket()
		_ = c.Close()
		fmt.Printf("READY %s %s\n", value.Id, socket)
		os.Stdout.Sync()
		time.Sleep(5 * time.Minute)
		return
	}
	if os.Getenv("LABCONTAINERS_LIVE") != "1" {
		t.Skip("requires privileged Containerlab")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	dir := t.TempDir()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLivePersistentReaperTearsDownWhenOwnerKilled$", "-test.v")
	cmd.Env = append(os.Environ(), "LABCONTAINERS_PERSISTENT_REAPER_CHILD=1", "LABCONTAINERS_PERSISTENT_REAPER_DIR="+dir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	var sessionID, socket string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if rest, ok := strings.CutPrefix(line, "READY "); ok {
			parts := strings.SplitN(rest, " ", 2)
			if len(parts) == 2 {
				sessionID, socket = parts[0], parts[1]
				break
			}
		}
	}
	if sessionID == "" || socket == "" {
		t.Fatalf("child did not report READY session")
	}

	containers, err := exec.CommandContext(ctx, "docker", "ps", "-q", "--filter", "label=labcontainers.appmana.com/session="+sessionID).Output()
	if err != nil || strings.TrimSpace(string(containers)) == "" {
		t.Fatalf("expected running container for session %s before kill: %q %v", sessionID, containers, err)
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill child: %v", err)
	}
	_ = cmd.Wait()

	for {
		out, err := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", "label=labcontainers.appmana.com/session="+sessionID).Output()
		_, sockErr := os.Stat(socket)
		if err == nil && strings.TrimSpace(string(out)) == "" && os.IsNotExist(sockErr) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("reaper did not tear down persistent session %s after child SIGKILL (containers=%q, socketErr=%v)", sessionID, strings.TrimSpace(string(out)), sockErr)
		case <-time.After(250 * time.Millisecond):
		}
	}
}
