package server

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/internal/engine"
	"github.com/appmana/labcontainers/internal/session"
)

type fakeBackend struct{ calls []string }

func (f *fakeBackend) call(value string)                          { f.calls = append(f.calls, value) }
func (f *fakeBackend) Doctor(context.Context) error               { f.call("doctor"); return nil }
func (f *fakeBackend) Validate(_ context.Context, _ string) error { f.call("validate"); return nil }
func (f *fakeBackend) Deploy(_ context.Context, _ string) error   { f.call("deploy"); return nil }
func (f *fakeBackend) ProofIsolation(_ context.Context, _ string, _ []string) error {
	f.call("proof")
	return nil
}
func (f *fakeBackend) Destroy(_ context.Context, _ string) error { f.call("destroy"); return nil }
func (f *fakeBackend) Lifecycle(_ context.Context, _, node, action string) error {
	f.call(action + ":" + node)
	return nil
}
func (f *fakeBackend) Replace(_ context.Context, _, node string) error {
	f.call("replace:" + node)
	return nil
}
func (f *fakeBackend) Exec(_ context.Context, _, node, _ string, _ time.Duration, _ []byte, argv []string) (engine.Result, error) {
	f.call("exec:" + node + ":" + argv[0])
	return engine.Result{Stdout: []byte("ok")}, nil
}
func (f *fakeBackend) Put(_ context.Context, _, node, _, path string, _ uint32, _ []byte) error {
	f.call("put:" + node + ":" + path)
	return nil
}
func (f *fakeBackend) SetLink(_ context.Context, _, node, _, iface string, up bool) error {
	f.call("link:" + node + ":" + iface + ":" + map[bool]string{true: "up", false: "down"}[up])
	return nil
}
func (f *fakeBackend) Netem(_ context.Context, node, iface string, _ time.Duration, _ time.Duration, _ float64, _ uint64, _ float64) error {
	f.call("netem:" + node + ":" + iface)
	return nil
}
func (f *fakeBackend) ResetNetem(_ context.Context, node, iface string) error {
	f.call("reset:" + node + ":" + iface)
	return nil
}

func testServer(t *testing.T) (*Server, *fakeBackend) {
	t.Helper()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{}
	return New(store, backend), backend
}

func createTestSession(t *testing.T, s *Server) *labv1.Session {
	t.Helper()
	p, err := s.CreateSession(context.Background(), &labv1.CreateSessionRequest{Spec: &labv1.LabSpec{Topology: &labv1.TopologySource{Source: &labv1.TopologySource_Yaml{Yaml: []byte(`name: example
topology:
  defaults: {network-mode: none}
  nodes:
    n1: {kind: linux, image: alpine:3}
`)}}}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSessionLifecycleAndFaultRollback(t *testing.T) {
	s, backend := testServer(t)
	p := createTestSession(t, s)
	if p.GetState() != "running" || len(p.GetNodes()) != 1 {
		t.Fatalf("session = %#v", p)
	}
	node := &labv1.NodeRef{SessionId: p.GetId(), Node: "n1"}
	if _, err := s.Lifecycle(context.Background(), &labv1.LifecycleRequest{Node: node, Action: labv1.LifecycleAction_POWER_OFF}); err != nil {
		t.Fatal(err)
	}
	f, err := s.ApplyFault(context.Background(), &labv1.ApplyFaultRequest{SessionId: p.GetId(), Fault: &labv1.ApplyFaultRequest_LinkState{LinkState: &labv1.LinkState{Node: "n1", Interface: "eth0", Up: false}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RevertFault(context.Background(), &labv1.FaultRef{SessionId: p.GetId(), Id: f.GetId()}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DestroySession(context.Background(), &labv1.DestroySessionRequest{Id: p.GetId(), ResumeToken: p.GetResumeToken()}); err != nil {
		t.Fatal(err)
	}
	wantTail := []string{"stop:n1", "link:n1:eth0:down", "link:n1:eth0:up", "destroy"}
	if got := backend.calls[len(backend.calls)-len(wantTail):]; !reflect.DeepEqual(got, wantTail) {
		t.Fatalf("calls = %#v, want tail %#v", backend.calls, wantTail)
	}
}

func TestTimelineRetainsDeclarationOrder(t *testing.T) {
	s, backend := testServer(t)
	p := createTestSession(t, s)
	ref := &labv1.NodeRef{SessionId: p.GetId(), Node: "n1"}
	result, err := s.RunTimeline(context.Background(), &labv1.RunTimelineRequest{SessionId: p.GetId(), Actions: []*labv1.TimelineAction{
		{AtMillis: 0, Action: &labv1.TimelineAction_Exec{Exec: &labv1.ExecRequest{Node: ref, Argv: []string{"first"}}}},
		{AtMillis: 0, Action: &labv1.TimelineAction_Exec{Exec: &labv1.ExecRequest{Node: ref, Argv: []string{"second"}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.GetCompleted() != 2 {
		t.Fatalf("completed %d", result.GetCompleted())
	}
	want := []string{"exec:n1:first", "exec:n1:second"}
	if got := backend.calls[len(backend.calls)-2:]; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestVMDisksAreSparseAndReplaceResetsThem(t *testing.T) {
	s, _ := testServer(t)
	p, err := s.CreateSession(context.Background(), &labv1.CreateSessionRequest{Spec: &labv1.LabSpec{
		Topology: &labv1.TopologySource{Source: &labv1.TopologySource_Yaml{Yaml: []byte("name: vm\ntopology:\n  defaults: {network-mode: none}\n  nodes:\n    vm: {kind: linux, image: vm:test}\n")}},
		Nodes:    map[string]*labv1.NodeExtension{"vm": {Control: "qga", Disks: []*labv1.Disk{{Name: "data", SizeBytes: 4096}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	record, err := s.Store.Get(p.GetId())
	if err != nil {
		t.Fatal(err)
	}
	disk := record.Nodes["vm"].Disks[0]
	info, err := os.Stat(disk.Path)
	if err != nil || info.Size() != 4096 {
		t.Fatalf("disk info=%v err=%v", info, err)
	}
	if err := os.WriteFile(disk.Path, []byte("dirty"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lifecycle(context.Background(), &labv1.LifecycleRequest{Node: &labv1.NodeRef{SessionId: p.GetId(), Node: "vm"}, Action: labv1.LifecycleAction_REPLACE}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(disk.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 4096 || contents[0] != 0 {
		t.Fatal("replace did not reset the disposable disk")
	}
}

func TestReplaceCanSupplyFirstBootDataAfterPoolCreation(t *testing.T) {
	s, _ := testServer(t)
	p, err := s.CreateSession(context.Background(), &labv1.CreateSessionRequest{Spec: &labv1.LabSpec{
		Topology: &labv1.TopologySource{Source: &labv1.TopologySource_Yaml{Yaml: []byte("name: vm\ntopology:\n  defaults: {network-mode: none}\n  nodes:\n    vm: {kind: linux, image: vm:test}\n")}},
		Nodes:    map[string]*labv1.NodeExtension{"vm": {Control: "qga"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("#cloud-config\nhostname: replaced\n")
	if _, err := s.Lifecycle(context.Background(), &labv1.LifecycleRequest{Node: &labv1.NodeRef{SessionId: p.GetId(), Node: "vm"}, Action: labv1.LifecycleAction_REPLACE, Bootstrap: &labv1.BootstrapData{Format: "cloud-config", Value: data}}); err != nil {
		t.Fatal(err)
	}
	record, err := s.Store.Get(p.GetId())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(record.Nodes["vm"].BootstrapPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("bootstrap = %q", got)
	}
	topology, err := os.ReadFile(record.TopologyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(topology), ":/extra-userdata.yaml:ro") {
		t.Fatalf("bootstrap bind absent:\n%s", topology)
	}
}
