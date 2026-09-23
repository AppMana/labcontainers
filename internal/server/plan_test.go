package server

import (
	"context"
	"os"
	"testing"

	labv1 "github.com/appmana/labcontainers/api/v1"
	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/types"
)

func TestPlanDraftPreservesAuthoritativeTopologyAndIsolation(t *testing.T) {
	s, backend := testServer(t)
	p := createTestSession(t, s)
	before, err := os.ReadFile(p.GetTopologyPath())
	if err != nil {
		t.Fatal(err)
	}
	config := &core.Config{Topology: &types.Topology{Nodes: map[string]*types.NodeDefinition{
		"new": {Kind: "linux", Image: "alpine:3.20"},
	}}}
	source, err := clab.Source(config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.PlanTopology(context.Background(), &labv1.PlanTopologyRequest{SessionId: p.GetId(), Topology: source})
	if err != nil || len(result.GetJson()) == 0 {
		t.Fatalf("result=%v error=%v", result, err)
	}
	after, err := os.ReadFile(p.GetTopologyPath())
	if err != nil || string(after) != string(before) {
		t.Fatal("plan changed authoritative topology")
	}
	if backend.calls[len(backend.calls)-1] != "plan" {
		t.Fatalf("calls=%v", backend.calls)
	}
	config.Topology.Nodes["new"].NetworkMode = "host"
	source, err = clab.Source(config)
	if err != nil {
		t.Fatal(err)
	}
	count := len(backend.calls)
	if _, err := s.PlanTopology(context.Background(), &labv1.PlanTopologyRequest{SessionId: p.GetId(), Topology: source}); err == nil {
		t.Fatal("accepted unsafe draft")
	}
	if len(backend.calls) != count {
		t.Fatal("unsafe draft reached runtime")
	}
}
