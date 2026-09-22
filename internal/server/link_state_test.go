package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"
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

func TestFaultWriteAheadAndRecoverableBackendFailure(t *testing.T) {
	s, backend := testServer(t)
	lab := createTestSession(t, s)
	backend.linkSetHook = func() error {
		record, err := s.Store.Get(lab.Id)
		if err != nil {
			t.Fatal(err)
		}
		if len(record.Faults) != 1 {
			t.Fatal("network mutation preceded durable rollback record")
		}
		for _, fault := range record.Faults {
			if !fault.Active || !fault.RestoreUp {
				t.Fatal("prior link state not retained")
			}
		}
		return errors.New("backend outcome uncertain")
	}
	_, err := s.ApplyFault(context.Background(), &labv1.ApplyFaultRequest{SessionId: lab.Id, Fault: &labv1.ApplyFaultRequest_LinkState{LinkState: &labv1.LinkState{Node: "n1", Interface: "eth0", Up: false}}})
	if err == nil {
		t.Fatal("lost backend failure")
	}
	var faultID string
	for _, detail := range status.Convert(err).Details() {
		if info, ok := detail.(*errdetails.ErrorInfo); ok {
			if info.Reason != "FAULT_APPLY_FAILED" || info.Metadata["session_id"] != lab.Id {
				t.Fatal("wrong recovery metadata")
			}
			faultID = info.Metadata["fault_id"]
		}
	}
	if faultID == "" {
		t.Fatal("backend error has no machine-readable rollback identity")
	}
	backend.linkSetHook = nil
	if _, err := s.RevertFault(context.Background(), &labv1.FaultRef{SessionId: lab.Id, Id: faultID}); err != nil {
		t.Fatal(err)
	}
	record, err := s.Store.Get(lab.Id)
	if err != nil {
		t.Fatal(err)
	}
	if record.Faults[faultID].Active || !backend.linkUp["n1:eth0"] {
		t.Fatal("explicit recovery failed")
	}
}

func TestFaultPersistenceFailureDoesNotMutateNetwork(t *testing.T) {
	s, backend := testServer(t)
	lab := createTestSession(t, s)
	backend.linkReadHook = func() {
		// Make the final atomic rename fail after the initial session read,
		// without depending on UID or filesystem permission enforcement.
		filename := filepath.Join(s.Store.SessionDir(lab.Id), "session.json")
		if err := os.Rename(filename, filename+".backup"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filename, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	backend.linkSetHook = func() error { t.Fatal("network changed after persistence failure"); return nil }
	_, err := s.ApplyFault(context.Background(), &labv1.ApplyFaultRequest{SessionId: lab.Id, Fault: &labv1.ApplyFaultRequest_LinkState{LinkState: &labv1.LinkState{Node: "n1", Interface: "eth0", Up: false}}})
	if err == nil {
		t.Fatal("ignored persistence failure")
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
