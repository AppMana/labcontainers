package server

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	labv1 "github.com/appmana/labcontainers/api/v1"
	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/types"
)

func TestApplyRequiresApprovedPlanAndRechecksIsolation(t *testing.T) {
	s, backend := testServer(t)
	p := createTestSession(t, s)
	source, err := clab.Source(&core.Config{Topology: &types.Topology{Nodes: map[string]*types.NodeDefinition{
		"new": {Kind: "linux", Image: "alpine:3.20"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p.GetTopologyPath())
	if err != nil {
		t.Fatal(err)
	}
	request := &labv1.ApplyTopologyRequest{SessionId: p.GetId(), Topology: source}
	if _, err := s.ApplyTopology(context.Background(), request); err == nil {
		t.Fatal("missing approval accepted")
	}
	request.ApprovedPlan = &labv1.NativeApplyResult{Json: []byte(`{"dry-run":true}`)}
	if _, err := s.ApplyTopology(context.Background(), request); err == nil || !strings.Contains(err.Error(), "plan changed") {
		t.Fatalf("stale approval: %v", err)
	}
	after, err := os.ReadFile(p.GetTopologyPath())
	if err != nil || string(before) != string(after) {
		t.Fatal("stale approval changed authoritative topology")
	}
	plan, err := s.PlanTopology(context.Background(), &labv1.PlanTopologyRequest{SessionId: p.GetId(), Topology: source})
	if err != nil {
		t.Fatal(err)
	}
	request.ApprovedPlan = plan
	backend.nameConflict = errors.New("foreign container")
	if _, err := s.ApplyTopology(context.Background(), request); err == nil {
		t.Fatal("foreign runtime accepted")
	}
	backend.nameConflict = nil
	result, err := s.ApplyTopology(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].Name != "new" {
		t.Fatalf("nodes=%v", result.Nodes)
	}
	if len(backend.proofNodes) != 1 || backend.proofNodes[0] != "new" {
		t.Fatal("new node isolation not verified")
	}
}

type failedApplyBackend struct {
	*fakeBackend
	stage string
}

func (b *failedApplyBackend) Deploy(ctx context.Context, path string) error {
	if b.stage == "deploy" {
		return errors.New("partial native failure")
	}
	return b.fakeBackend.Deploy(ctx, path)
}

func (b *failedApplyBackend) ProofIsolation(ctx context.Context, lab string, nodes []string) error {
	if b.stage == "isolation" {
		return errors.New("unexpected runtime network")
	}
	return b.fakeBackend.ProofIsolation(ctx, lab, nodes)
}

func TestApplyRetainsPartialFailureForExplicitRecovery(t *testing.T) {
	for _, stage := range []string{"deploy", "isolation"} {
		t.Run(stage, func(t *testing.T) {
			s, backend := testServer(t)
			p := createTestSession(t, s)
			s.Backend = &failedApplyBackend{fakeBackend: backend, stage: stage}
			source, err := clab.Source(&core.Config{Topology: &types.Topology{Nodes: map[string]*types.NodeDefinition{
				"new": {Kind: "linux", Image: "alpine:3.20"},
			}}})
			if err != nil {
				t.Fatal(err)
			}
			approved, err := s.PlanTopology(context.Background(), &labv1.PlanTopologyRequest{SessionId: p.GetId(), Topology: source})
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.ApplyTopology(context.Background(), &labv1.ApplyTopologyRequest{SessionId: p.GetId(), Topology: source, ApprovedPlan: approved})
			if err == nil || !strings.Contains(err.Error(), "partial state retained") {
				t.Fatalf("failure was hidden: %v", err)
			}
			r, err := s.Store.Get(p.GetId())
			if err != nil {
				t.Fatal(err)
			}
			if r.State != "reconcile-failed" || r.Nodes["new"] == nil || r.Nodes["n1"] == nil {
				t.Fatalf("lost partial state: %+v", r)
			}
			if _, err := s.DestroySession(context.Background(), &labv1.DestroySessionRequest{Id: p.GetId()}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
