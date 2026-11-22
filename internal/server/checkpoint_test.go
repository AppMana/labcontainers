package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/pkg/checkpoint"
	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func vmSession(t *testing.T, s *Server) *labv1.Session {
	t.Helper()
	source, err := clab.Source(&core.Config{Topology: &types.Topology{Nodes: map[string]*types.NodeDefinition{
		"vm":     {Kind: "generic_vm", Image: "prepared-vm"},
		"switch": {Kind: "linux", Image: "prepared-container"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateSession(context.Background(), &labv1.CreateSessionRequest{Spec: &labv1.LabSpec{
		Topology: source, Nodes: map[string]*labv1.NodeExtension{"vm": {Control: "qga", Disks: []*labv1.Disk{{Name: "data", SizeBytes: 8}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func stop(t *testing.T, s *Server, id, node string) {
	t.Helper()
	if _, err := s.Lifecycle(context.Background(), &labv1.LifecycleRequest{Node: &labv1.NodeRef{SessionId: id, Node: node}, Action: labv1.LifecycleAction_POWER_OFF}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointRequiresStoppedNodesAndNeverOverwrites(t *testing.T) {
	s, backend := testServer(t)
	p := vmSession(t, s)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "from")
	req := &labv1.CheckpointRequest{SessionId: p.GetId(), Directory: dir, Labels: map[string]string{"set": "from"}}
	if _, err := s.CreateCheckpoint(ctx, req); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("checkpointed a running node: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("a refused checkpoint left files behind")
	}
	stop(t, s, p.GetId(), "vm")
	record, err := s.Store.Get(p.GetId())
	if err != nil {
		t.Fatal(err)
	}
	disk := record.Nodes["vm"].Disks[0].Path
	if err := os.WriteFile(disk, []byte("provisnd"), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateCheckpoint(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.GetNodes()) != 1 || created.GetNodes()[0].GetName() != "vm" || created.GetLabels()["set"] != "from" {
		t.Fatalf("unexpected checkpoint: %v", created)
	}
	manifest, err := checkpoint.Read(dir)
	if err != nil || manifest.Nodes["vm"].Image != "sha256:image-vm" || manifest.Session != p.GetId() {
		t.Fatalf("manifest: %+v %v", manifest, err)
	}
	if _, err := s.CreateCheckpoint(ctx, req); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("overwrote an existing checkpoint: %v", err)
	}
	for _, bad := range []string{"relative", "/"} {
		if _, err := s.CreateCheckpoint(ctx, &labv1.CheckpointRequest{SessionId: p.GetId(), Directory: bad}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("directory %q: %v", bad, err)
		}
	}

	// The provisioned content comes back after later writes, in this session
	// or in another one built from the same topology.
	if err := os.WriteFile(disk, []byte("upgraded"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend.calls = nil
	restored, err := s.RestoreCheckpoint(ctx, &labv1.CheckpointRequest{SessionId: p.GetId(), Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(disk); string(data) != "provisnd" {
		t.Fatalf("data disk holds %q", data)
	}
	if restored.GetSizeBytes() == 0 || strings.Join(backend.calls, ";") != "import vm sha256:image-vm base-overlay.qcow2;copy-disk" {
		t.Fatalf("restore: %v %v", restored, backend.calls)
	}
	other := vmSession(t, s)
	if _, err := s.RestoreCheckpoint(ctx, &labv1.CheckpointRequest{SessionId: other.GetId(), Directory: dir}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("restored into a running node: %v", err)
	}
	stop(t, s, other.GetId(), "vm")
	if _, err := s.RestoreCheckpoint(ctx, &labv1.CheckpointRequest{SessionId: other.GetId(), Directory: dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestoreCheckpoint(ctx, &labv1.CheckpointRequest{SessionId: other.GetId(), Directory: t.TempDir()}); status.Code(err) != codes.NotFound {
		t.Fatalf("restored from an empty directory: %v", err)
	}
}

func TestRestoreRejectsDifferentDiskLayoutBeforeChangingAnything(t *testing.T) {
	s, backend := testServer(t)
	p := vmSession(t, s)
	ctx := context.Background()
	stop(t, s, p.GetId(), "vm")
	dir := filepath.Join(t.TempDir(), "from")
	if _, err := s.CreateCheckpoint(ctx, &labv1.CheckpointRequest{SessionId: p.GetId(), Directory: dir}); err != nil {
		t.Fatal(err)
	}
	record, err := s.Store.Get(p.GetId())
	if err != nil {
		t.Fatal(err)
	}
	record.Nodes["vm"].Disks[0].SizeBytes = 16
	if err := s.Store.Save(record); err != nil {
		t.Fatal(err)
	}
	backend.calls = nil
	if _, err := s.RestoreCheckpoint(ctx, &labv1.CheckpointRequest{SessionId: p.GetId(), Directory: dir}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("accepted a different disk layout: %v", err)
	}
	if len(backend.calls) != 0 {
		t.Fatalf("changed disks before validation finished: %v", backend.calls)
	}
}

func TestExtendMovesLeaseLaterWithoutKeeping(t *testing.T) {
	s, _ := testServer(t)
	p := createTestSession(t, s)
	ctx := context.Background()
	if _, err := s.ExtendSession(ctx, &labv1.ExtendSessionRequest{Id: p.GetId()}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("accepted a renewal without a ttl: %v", err)
	}
	longer, err := s.ExtendSession(ctx, &labv1.ExtendSessionRequest{Id: p.GetId(), TtlSeconds: int64((6 * time.Hour).Seconds())})
	if err != nil {
		t.Fatal(err)
	}
	if longer.GetExpiresUnix() <= p.GetExpiresUnix() {
		t.Fatal("lease did not move")
	}
	shorter, err := s.ExtendSession(ctx, &labv1.ExtendSessionRequest{Id: p.GetId(), TtlSeconds: 60})
	if err != nil || shorter.GetExpiresUnix() != longer.GetExpiresUnix() {
		t.Fatalf("a short renewal cut the lease: %v %v", shorter.GetExpiresUnix(), err)
	}
	// Still owned: the owner's exit destroys an extended session.
	if err := s.CleanupUnkept(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, &labv1.SessionRef{Id: p.GetId()}); status.Code(err) != codes.NotFound {
		t.Fatalf("an extended session survived its owner: %v", err)
	}
	// A lease that keeps being renewed is never scavenged; one that stops is.
	q := createTestSession2(t, s)
	if err := s.Scavenge(ctx, time.Now().Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, &labv1.SessionRef{Id: q.GetId()}); status.Code(err) != codes.NotFound {
		t.Fatal("an unrenewed lease outlived its ttl")
	}
}

func createTestSession2(t *testing.T, s *Server) *labv1.Session {
	t.Helper()
	p, err := s.CreateSession(context.Background(), &labv1.CreateSessionRequest{Spec: &labv1.LabSpec{Name: "second", Topology: &labv1.TopologySource{Source: &labv1.TopologySource_Yaml{Yaml: []byte("topology:\n  defaults: {network-mode: none}\n  nodes:\n    n1: {kind: linux, image: alpine:3}\n")}}}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
