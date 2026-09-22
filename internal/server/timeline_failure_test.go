package server

import (
	"context"
	"strings"
	"testing"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/internal/engine"
)

func TestTimelineUnobservedEventNeverCutsPower(t *testing.T) {
	s, backend := testServer(t)
	session := createTestSession(t, s)
	ref := &labv1.NodeRef{Node: "n1"}
	_, err := s.RunTimeline(context.Background(), &labv1.RunTimelineRequest{SessionId: session.GetId(), Actions: []*labv1.TimelineAction{
		{Action: &labv1.TimelineAction_WaitExec{WaitExec: &labv1.WaitExec{Exec: &labv1.ExecRequest{Node: ref, Argv: []string{"probe"}}, TimeoutMillis: 20, RetryMillis: 1, StdoutContains: []byte("never-observed")}}},
		{Action: &labv1.TimelineAction_Lifecycle{Lifecycle: &labv1.LifecycleRequest{Node: ref, Action: labv1.LifecycleAction_POWER_OFF}}},
	}})
	if err == nil {
		t.Fatal("unobserved event passed")
	}
	for _, call := range backend.calls {
		if strings.HasPrefix(call, "stop:") {
			t.Fatalf("power cut despite missing event: %v", backend.calls)
		}
	}
}

func TestTimelineFailedExecStopsBeforePowerCut(t *testing.T) {
	s, backend := testServer(t)
	session := createTestSession(t, s)
	backend.execResults = []engine.Result{{ExitCode: 7}}
	ref := &labv1.NodeRef{Node: "n1"}
	_, err := s.RunTimeline(context.Background(), &labv1.RunTimelineRequest{SessionId: session.GetId(), Actions: []*labv1.TimelineAction{
		{Action: &labv1.TimelineAction_Exec{Exec: &labv1.ExecRequest{Node: ref, Argv: []string{"failed-setup"}}}},
		{Action: &labv1.TimelineAction_Lifecycle{Lifecycle: &labv1.LifecycleRequest{Node: ref, Action: labv1.LifecycleAction_POWER_OFF}}},
	}})
	if err == nil {
		t.Fatal("failed setup accepted as successful timeline")
	}
	for _, call := range backend.calls {
		if strings.HasPrefix(call, "stop:") {
			t.Fatalf("power cut after failed setup: %v", backend.calls)
		}
	}
}
