package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/internal/engine"
	"github.com/appmana/labcontainers/internal/session"
	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/types"
	"gopkg.in/yaml.v2"
)

type fakeBackend struct {
	runtimeErr   error
	nameConflict error
	calls        []string
	execResults  []engine.Result
	proofNodes   []string
	removeHook   func() error
}

func (f *fakeBackend) CheckLabNameAvailable(context.Context, string) error {
	return f.nameConflict
}
func (f *fakeBackend) CheckSessionOwnership(context.Context, string, string) error {
	return f.nameConflict
}
func (f *fakeBackend) RestoreAttachments(context.Context, string, string) error { return nil }

func (f *fakeBackend) call(value string)                          { f.calls = append(f.calls, value) }
func (f *fakeBackend) Doctor(context.Context) error               { f.call("doctor"); return nil }
func (f *fakeBackend) Validate(_ context.Context, _ string) error { f.call("validate"); return nil }
func (f *fakeBackend) Deploy(_ context.Context, _ string) error   { f.call("deploy"); return nil }
func (f *fakeBackend) Plan(_ context.Context, _ string) ([]byte, error) {
	f.call("plan")
	return []byte(`{"dry-run":true,"added-nodes":["new"]}`), nil
}
func (f *fakeBackend) ContainerName(_ context.Context, lab, node string) (string, error) {
	if f.runtimeErr != nil {
		return "", f.runtimeErr
	}
	return "native-" + lab + "-" + node, nil
}
func (f *fakeBackend) ProofIsolation(_ context.Context, _ string, nodes []string) error {
	f.call("proof")
	f.proofNodes = append([]string(nil), nodes...)
	return nil
}

func TestExternalOptInRetainsIsolationChecks(t *testing.T) {
	s, backend := testServer(t)
	source, err := clab.Source(&core.Config{Name: "mixed", Topology: &types.Topology{
		Nodes: map[string]*types.NodeDefinition{
			"isolated": {Kind: "linux", Image: "alpine:3.20"},
			"wan":      {Kind: "linux", Image: "alpine:3.20", NetworkMode: "host"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateSession(context.Background(), &labv1.CreateSessionRequest{Spec: &labv1.LabSpec{Topology: source, AllowExternalAccess: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(backend.proofNodes, []string{"isolated"}) {
		t.Fatalf("unexpected runtime checks: %v", backend.proofNodes)
	}
}

func TestCreateDoesNotReconcileAnExistingForeignLab(t *testing.T) {
	s, backend := testServer(t)
	backend.nameConflict = errors.New("name already used by another daemon")
	source, err := clab.Source(&core.Config{Topology: &types.Topology{
		Nodes: map[string]*types.NodeDefinition{"n1": {Kind: "linux", Image: "alpine:3.20"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateSession(context.Background(), &labv1.CreateSessionRequest{
		Spec: &labv1.LabSpec{Name: "existing", Topology: source},
	})
	if err == nil || !strings.Contains(err.Error(), "name already used") {
		t.Fatalf("expected ownership preflight failure, got %v", err)
	}
	if !reflect.DeepEqual(backend.calls, []string{"doctor"}) {
		t.Fatalf("mutated foreign runtime: %v", backend.calls)
	}
	records, err := s.Store.List()
	if err != nil || len(records) != 0 {
		t.Fatalf("created local session for foreign lab: %v %v", records, err)
	}
}

func TestInMemoryTopologyRetainsRelativeBindContext(t *testing.T) {
	s, _ := testServer(t)
	base := t.TempDir()
	source, err := clab.Source(&core.Config{Name: "binds", Topology: &types.Topology{
		Nodes: map[string]*types.NodeDefinition{"node": {Kind: "linux", Image: "alpine:3.20", Binds: []string{"data:/data:ro"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	source.BaseDirectory = base
	session, err := s.CreateSession(context.Background(), &labv1.CreateSessionRequest{Spec: &labv1.LabSpec{Topology: source}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(session.GetTopologyPath())
	if err != nil {
		t.Fatal(err)
	}
	var config core.Config
	if err := yaml.UnmarshalStrict(raw, &config); err != nil {
		t.Fatal(err)
	}
	if got, want := config.Topology.Nodes["node"].Binds[0], filepath.Join(base, "data")+":/data:ro"; got != want {
		t.Fatalf("bind = %q, want %q", got, want)
	}
	source.BaseDirectory = "relative"
	if _, _, err := topologyBytes(source); err == nil {
		t.Fatal("relative base directory accepted")
	}
	source.Source = &labv1.TopologySource_Path{Path: "file.yml"}
	if _, _, err := topologyBytes(source); err == nil {
		t.Fatal("ambiguous source directory accepted")
	}
}
func (f *fakeBackend) Destroy(_ context.Context, _ string) error { f.call("destroy"); return nil }
func (f *fakeBackend) Lifecycle(_ context.Context, _, node, action string) error {
	f.call(action + ":" + node)
	return nil
}
func (f *fakeBackend) RemoveNode(_ context.Context, _, node string) error {
	f.call("remove:" + node)
	if f.removeHook != nil {
		return f.removeHook()
	}
	return nil
}
func (f *fakeBackend) Exec(_ context.Context, _, node, _ string, _ time.Duration, _ []byte, argv []string) (engine.Result, error) {
	f.call("exec:" + node + ":" + argv[0])
	if len(f.execResults) != 0 {
		result := f.execResults[0]
		f.execResults = f.execResults[1:]
		return result, nil
	}
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
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
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

func TestTimelineWaitExecTriggersLifecycleAfterObservedEvent(t *testing.T) {
	s, backend := testServer(t)
	p := createTestSession(t, s)
	ref := &labv1.NodeRef{SessionId: p.GetId(), Node: "n1"}
	backend.execResults = []engine.Result{
		{ExitCode: 1},
		{ExitCode: 1},
		{ExitCode: 0, Stdout: []byte("copy-active\n")},
	}
	result, err := s.RunTimeline(context.Background(), &labv1.RunTimelineRequest{SessionId: p.GetId(), Actions: []*labv1.TimelineAction{
		{Action: &labv1.TimelineAction_WaitExec{WaitExec: &labv1.WaitExec{
			Exec:        &labv1.ExecRequest{Node: ref, Argv: []string{"observe-copy"}},
			RetryMillis: 1, TimeoutMillis: 1000, StdoutContains: []byte("copy-active"),
		}}},
		{Action: &labv1.TimelineAction_Lifecycle{Lifecycle: &labv1.LifecycleRequest{Node: ref, Action: labv1.LifecycleAction_POWER_OFF}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.GetCompleted() != 2 {
		t.Fatalf("completed %d", result.GetCompleted())
	}
	want := []string{"exec:n1:observe-copy", "exec:n1:observe-copy", "exec:n1:observe-copy", "stop:n1"}
	if got := backend.calls[len(backend.calls)-len(want):]; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestVMDisksAreSparseAndReplaceResetsThem(t *testing.T) {
	s, backend := testServer(t)
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
	backend.removeHook = func() error { return errors.New("node removal failed") }
	request := &labv1.LifecycleRequest{Node: &labv1.NodeRef{SessionId: p.GetId(), Node: "vm"}, Action: labv1.LifecycleAction_REPLACE}
	if _, err := s.Lifecycle(context.Background(), request); err == nil {
		t.Fatal("ignored failure to remove the old VM")
	}
	if data, err := os.ReadFile(disk.Path); err != nil || string(data) != "dirty" {
		t.Fatal("modified disks still held by the old VM")
	}
	backend.removeHook = func() error {
		if data, err := os.ReadFile(disk.Path); err != nil || string(data) != "dirty" {
			t.Fatal("reset disk before node removal")
		}
		return nil
	}
	before := len(backend.calls)
	node, err := s.Lifecycle(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if node.GetState() != "replacement-pending" {
		t.Fatalf("reported an unstarted replacement as %q", node.GetState())
	}
	if got := backend.calls[before:]; !reflect.DeepEqual(got, []string{"remove:vm"}) {
		t.Fatalf("replacement silently performed additional operations: %v", got)
	}
	contents, err := os.ReadFile(disk.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 4096 || contents[0] != 0 {
		t.Fatal("replace did not reset the disposable disk")
	}
}

func TestReplacementPreconditionsDoNotRemoveNodes(t *testing.T) {
	for _, scenario := range []string{"foreign runtime", "active fault", "invalid bootstrap", "non-qga bootstrap"} {
		t.Run(scenario, func(t *testing.T) {
			s, backend := testServer(t)
			p := createTestSession(t, s)
			req := &labv1.LifecycleRequest{Node: &labv1.NodeRef{SessionId: p.GetId(), Node: "n1"}, Action: labv1.LifecycleAction_REPLACE}
			switch scenario {
			case "foreign runtime":
				backend.nameConflict = errors.New("foreign runtime")
			case "active fault":
				r, err := s.Store.Get(p.GetId())
				if err != nil {
					t.Fatal(err)
				}
				r.Faults = map[string]*session.Fault{"test": {ID: "test", Active: true}}
				if err := s.Store.Save(r); err != nil {
					t.Fatal(err)
				}
			case "invalid bootstrap":
				req.Bootstrap = &labv1.BootstrapData{Format: "not-a-format", Value: []byte("invalid")}
			case "non-qga bootstrap":
				req.Bootstrap = &labv1.BootstrapData{Format: "cloud-config", Value: []byte("#cloud-config\n")}
			}
			before := len(backend.calls)
			if _, err := s.Lifecycle(context.Background(), req); err == nil {
				t.Fatal("invalid replacement accepted")
			}
			if len(backend.calls) != before {
				t.Fatalf("precondition failure touched nodes: %v", backend.calls[before:])
			}
		})
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
