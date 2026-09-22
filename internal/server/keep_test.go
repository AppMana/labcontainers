package server

import (
	"context"
	"testing"

	labv1 "github.com/appmana/labcontainers/api/v1"
)

func TestAutomaticCleanupHonorsRawKeepButExplicitDestroyDoesNot(t *testing.T) {
	s, backend := testServer(t)
	p := createTestSession(t, s)
	ctx := context.Background()
	if _, err := s.KeepSession(ctx, &labv1.KeepSessionRequest{Id: p.GetId(), TtlSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	before := len(backend.calls)
	req := &labv1.DestroySessionRequest{Id: p.GetId(), ResumeToken: p.GetResumeToken(), PreserveKept: true}
	if _, err := s.DestroySession(ctx, req); err != nil {
		t.Fatal(err)
	}
	if len(backend.calls) != before {
		t.Fatal("automatic cleanup destroyed kept runtime")
	}
	if _, err := s.GetSession(ctx, &labv1.SessionRef{Id: p.GetId()}); err != nil {
		t.Fatal("kept state lost", err)
	}
	req.PreserveKept = false
	if _, err := s.DestroySession(ctx, req); err != nil {
		t.Fatal(err)
	}
	if len(backend.calls) != before+1 || backend.calls[before] != "destroy" {
		t.Fatal("explicit destroy was skipped")
	}
}
