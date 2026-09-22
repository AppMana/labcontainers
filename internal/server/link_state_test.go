package server

import (
	"context"
	"errors"
	"testing"

	labv1 "github.com/appmana/labcontainers/api/v1"
)

func TestLinkFaultRestoresObservedState(t *testing.T) {
	for _, prior := range []bool{false, true} {
		for _, requested := range []bool{false, true} {
			s, backend := testServer(t)
			lab := createTestSession(t, s)
			backend.linkUp = map[string]bool{"n1:eth0": prior}
			request := &labv1.ApplyFaultRequest{SessionId: lab.Id, Fault: &labv1.ApplyFaultRequest_LinkState{LinkState: &labv1.LinkState{Node: "n1", Interface: "eth0", Up: requested}}}
			fault, err := s.ApplyFault(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if backend.linkUp["n1:eth0"] != requested {
				t.Fatal("requested fault not applied")
			}
			if _, err := s.ApplyFault(context.Background(), request); err == nil {
				t.Fatal("accepted overlapping fault with ambiguous restoration order")
			}
			if _, err := s.RevertFault(context.Background(), &labv1.FaultRef{SessionId: lab.Id, Id: fault.Id}); err != nil {
				t.Fatal(err)
			}
			if backend.linkUp["n1:eth0"] != prior {
				t.Fatalf("prior=%v requested=%v reverted=%v", prior, requested, backend.linkUp["n1:eth0"])
			}
		}
	}
}

func TestFailedLinkObservationDoesNotMutate(t *testing.T) {
	s, backend := testServer(t)
	lab := createTestSession(t, s)
	backend.linkReadErr = errors.New("endpoint missing")
	before := len(backend.calls)
	_, err := s.ApplyFault(context.Background(), &labv1.ApplyFaultRequest{SessionId: lab.Id, Fault: &labv1.ApplyFaultRequest_LinkState{LinkState: &labv1.LinkState{Node: "n1", Interface: "eth0", Up: false}}})
	if err == nil || len(backend.calls) != before {
		t.Fatal("fault mutated endpoint without observing prior state")
	}
}
