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

func TestLiveReaperTearsDownWhenOwnerKilled(t *testing.T) {
	if os.Getenv("LABCONTAINERS_LIVE") == "" {
		t.Skip("requires privileged Containerlab")
	}
	if os.Getenv("LABCONTAINERS_REAPER_CHILD") == "1" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		c, err := Launch(ctx, Options{LabdPath: os.Getenv("LABCONTAINERS_REAPER_LABD")})
		if err != nil {
			panic(err)
		}
		source, err := clab.Source(&core.Config{Topology: &types.Topology{Nodes: map[string]*types.NodeDefinition{
			"reaped": {Kind: "linux", Image: "alpine:3.20", ImagePullPolicy: "Never", NetworkMode: "none"},
		}}})
		if err != nil {
			panic(err)
		}
		lab, err := c.Start(ctx, &labv1.LabSpec{Topology: source}, time.Hour)
		if err != nil {
			panic(err)
		}
		info := lab.ID() + "\n" + c.Socket() + "\n" + lab.Artifacts() + "\n"
		if err := os.WriteFile(os.Getenv("LABCONTAINERS_REAPER_INFO"), []byte(info), 0o600); err != nil {
			panic(err)
		}
		// Block forever until parent sends SIGKILL without Close() or Destroy().
		select {}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	infoFile := filepath.Join(t.TempDir(), "reaper.info")
	child := exec.CommandContext(ctx, self, "-test.run", "^TestLiveReaperTearsDownWhenOwnerKilled$")
	child.Env = append(os.Environ(),
		"LABCONTAINERS_REAPER_CHILD=1",
		"LABCONTAINERS_REAPER_LABD="+filepath.Join(root, "bin/labd"),
		"LABCONTAINERS_REAPER_INFO="+infoFile,
	)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if child.Process != nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()

	var lines []string
	for {
		if raw, err := os.ReadFile(infoFile); err == nil {
			parts := strings.Split(strings.TrimSpace(string(raw)), "\n")
			if len(parts) == 3 {
				lines = parts
				break
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("child did not deploy session before timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}
	sessionID, socket, artifacts := lines[0], lines[1], lines[2]

	// Abruptly SIGKILL the owning test process without calling Close().
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()

	// Wait for the Ryuk-style labd reaper to tear down the topology and exit.
	for {
		out, err := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", "label=labcontainers.appmana.com/session="+sessionID).Output()
		if err == nil && strings.TrimSpace(string(out)) == "" {
			if _, sockErr := os.Stat(socket); os.IsNotExist(sockErr) {
				break
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("reaper did not tear down session %s after owner SIGKILL", sessionID)
		case <-time.After(100 * time.Millisecond):
		}
	}
	if _, err := os.Stat(filepath.Join(artifacts, "events.jsonl")); err != nil {
		t.Fatalf("reaper erased evidence: %v", err)
	}
}
