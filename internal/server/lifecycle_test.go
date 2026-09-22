package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	labv1 "github.com/appmana/labcontainers/api/v1"
)

func TestNativeStartCannotClaimMissingContainerIsRunning(t *testing.T) {
	s, backend := testServer(t)
	p := createTestSession(t, s)
	backend.runtimeErr = errors.New("no running container")
	_, err := s.Lifecycle(context.Background(), &labv1.LifecycleRequest{
		Node: &labv1.NodeRef{SessionId: p.GetId(), Node: "n1"}, Action: labv1.LifecycleAction_START,
	})
	if err == nil || !strings.Contains(err.Error(), "explicit recovery") {
		t.Fatalf("missing runtime was accepted: %v", err)
	}
	r, err := s.Store.Get(p.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if r.Nodes["n1"].State != "unknown" {
		t.Fatalf("unverified state=%q", r.Nodes["n1"].State)
	}
	if backend.calls[len(backend.calls)-1] != "start:n1" {
		t.Fatalf("unexpected fallback: %v", backend.calls)
	}
}

func TestStartRechecksTargetIsolation(t *testing.T) {
	s, backend := testServer(t)
	p := createTestSession(t, s)
	backend.proofNodes = nil
	_, err := s.Lifecycle(context.Background(), &labv1.LifecycleRequest{
		Node: &labv1.NodeRef{SessionId: p.GetId(), Node: "n1"}, Action: labv1.LifecycleAction_START,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(backend.proofNodes) != 1 || backend.proofNodes[0] != "n1" {
		t.Fatal("isolation not rechecked")
	}
}
