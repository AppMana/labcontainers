package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/internal/engine"
)

type deadlineBackend struct {
	*fakeBackend
	attempts int
}

func (b *deadlineBackend) Exec(ctx context.Context, _, _, _ string, _ time.Duration, _ []byte, _ []string) (engine.Result, error) {
	b.attempts++
	if b.attempts == 1 {
		return engine.Result{ExitCode: 125, Stderr: []byte("QGA handshake not ready")}, nil
	}
	<-ctx.Done()
	return engine.Result{ExitCode: -1}, nil
}

func TestWaitKeepsLastDiagnosticAndCallerObjects(t *testing.T) {
	s, backend := testServer(t)
	p := createTestSession(t, s)
	s.Backend = &deadlineBackend{fakeBackend: backend}
	predicate := &labv1.ExecRequest{Node: &labv1.NodeRef{Node: "n1"}, Argv: []string{"ready"}}
	_, err := s.RunTimeline(context.Background(), &labv1.RunTimelineRequest{
		SessionId: p.GetId(), Actions: []*labv1.TimelineAction{{
			Action: &labv1.TimelineAction_WaitExec{WaitExec: &labv1.WaitExec{
				Exec: predicate, TimeoutMillis: 100, RetryMillis: 1,
			}},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "QGA handshake not ready") {
		t.Fatalf("lost last meaningful attempt: %v", err)
	}
	if predicate.Node.SessionId != "" {
		t.Fatal("mutated caller-owned predicate")
	}
	events, err := os.ReadFile(filepath.Join(p.GetArtifactDirectory(), "events.jsonl"))
	if err != nil || !strings.Contains(string(events), "QGA handshake not ready") || !strings.Contains(string(events), "timeline.failed") {
		t.Fatalf("failure evidence not retained: %s %v", events, err)
	}
}
