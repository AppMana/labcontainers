package server

import (
	"testing"

	"github.com/appmana/labcontainers/internal/session"
)

func TestFaultInspectionPreservesFalseAndAbsentRestoreStates(t *testing.T) {
	r := &session.Record{Faults: map[string]*session.Fault{
		"z": {ID: "z", Kind: "netem", Node: "guest", Interface: "eth2", Active: true},
		"a": {ID: "a", Kind: "link-state", Node: "guest", Interface: "eth1", RestoreUp: false, Active: true},
	}}
	got := protoSession(r).Faults
	if len(got) != 2 || got[0].Id != "a" || got[1].Id != "z" {
		t.Fatalf("nondeterministic inspection: %v", got)
	}
	if got[0].RestoreUp == nil || got[0].GetRestoreUp() || got[1].RestoreUp != nil {
		t.Fatalf("absent and false restoration state conflated: %v", got)
	}
	*got[0].RestoreUp = true
	if r.Faults["a"].RestoreUp {
		t.Fatal("inspection aliases stored state")
	}
}
