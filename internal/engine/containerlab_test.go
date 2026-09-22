package engine

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	result Result
	argv   [][]string
}

func (f *fakeRunner) Run(_ context.Context, _ io.Reader, argv ...string) (Result, error) {
	f.argv = append(f.argv, append([]string(nil), argv...))
	return f.result, nil
}

func TestDoctorRequiresPinnedVersion(t *testing.T) {
	f := &fakeRunner{result: Result{Stdout: []byte("    version: 0.79.0\n")}}
	c := &Containerlab{Runner: f, Binary: "containerlab", Sudo: true}
	if err := c.Doctor(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := f.argv[0]; !reflect.DeepEqual(got, []string{"containerlab", "version"}) {
		t.Fatalf("doctor command = %#v", got)
	}
	f.result.Stdout = []byte("    version: 0.75.0\n")
	if err := c.Doctor(context.Background()); err == nil {
		t.Fatal("expected version mismatch")
	}
}

func TestReplaceUsesFilteredDestroyThenConvergence(t *testing.T) {
	f := &fakeRunner{}
	c := &Containerlab{Runner: f, Binary: "clab"}
	if err := c.Replace(context.Background(), "lab.clab.yml", "n1"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"clab", "destroy", "--topo", "lab.clab.yml", "--node-filter", "n1"}, {"clab", "deploy", "--topo", "lab.clab.yml", "--format", "json"}}
	if !reflect.DeepEqual(f.argv, want) {
		t.Fatalf("commands = %#v, want %#v", f.argv, want)
	}
}

func TestStartConvergesAfterGenericVMWasAutoRemoved(t *testing.T) {
	f := &fakeRunner{}
	c := &Containerlab{Runner: f, Binary: "clab"}
	if err := c.Lifecycle(context.Background(), "lab.clab.yml", "n1", "start"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"clab", "start", "--topo", "lab.clab.yml", "--node", "n1"}, {"clab", "deploy", "--topo", "lab.clab.yml", "--format", "json"}}
	if !reflect.DeepEqual(f.argv, want) {
		t.Fatalf("commands = %#v, want %#v", f.argv, want)
	}
}

func TestDestroyRemovesOnlyContainersOwnedByTopology(t *testing.T) {
	f := &fakeRunner{result: Result{Stdout: []byte("vm-id\npeer-id\n")}}
	c := &Containerlab{Runner: f, Binary: "clab"}
	if err := c.Destroy(context.Background(), "/tmp/session/topology.clab.yml"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"clab", "destroy", "--topo", "/tmp/session/topology.clab.yml", "--cleanup"},
		{"docker", "ps", "-aq", "--filter", "label=clab-topo-file=/tmp/session/topology.clab.yml"},
		{"docker", "rm", "-f", "vm-id", "peer-id"},
	}
	if !reflect.DeepEqual(f.argv, want) {
		t.Fatalf("commands = %#v, want %#v", f.argv, want)
	}
}

func TestProofIsolationReadsRuntimeNetworkState(t *testing.T) {
	f := &fakeRunner{result: Result{Stdout: []byte(`[{"HostConfig":{"NetworkMode":"none"},"NetworkSettings":{"Networks":{"none":{"EndpointID":"opaque"}}}}]`)}}
	c := &Containerlab{Runner: f, Binary: "clab"}
	if err := c.ProofIsolation(context.Background(), "lab", []string{"n1"}); err != nil {
		t.Fatal(err)
	}
	f.result.Stdout = []byte(`[{"HostConfig":{"NetworkMode":"bridge"},"NetworkSettings":{"Networks":{"bridge":{"IPAddress":"172.17.0.2"}}}}]`)
	if err := c.ProofIsolation(context.Background(), "lab", []string{"n1"}); err == nil {
		t.Fatal("accepted an attached runtime network")
	}
}

func TestQGAExecUsesStandaloneGuestBinary(t *testing.T) {
	f := &fakeRunner{}
	c := &Containerlab{Runner: f, Binary: "clab"}
	if _, err := c.Exec(context.Background(), "lab", "n1", "qga", time.Second, nil, []string{"true"}); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(f.argv[0], " ")
	if !strings.Contains(got, "/labcontainers-guest exec 1s 0 true") {
		t.Fatalf("command = %q", got)
	}
}

func TestPutPreservesPathsWithSpaces(t *testing.T) {
	f := &fakeRunner{}
	c := &Containerlab{Runner: f, Binary: "clab"}
	if err := c.Put(context.Background(), "lab", "n1", "container", "/var/lib/lab data/file", 0o600, []byte("x")); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(f.argv[0], " ")
	if !strings.Contains(got, `mkdir -p -- "$(dirname -- '/var/lib/lab data/file')"`) {
		t.Fatalf("put command = %q", got)
	}
}

func TestQGAPutUsesGuestHelper(t *testing.T) {
	f := &fakeRunner{}
	c := &Containerlab{Runner: f, Binary: "clab"}
	if err := c.Put(context.Background(), "lab", "win", "qga", `C:\Lab Data\file.txt`, 0o600, []byte("x")); err != nil {
		t.Fatal(err)
	}
	want := []string{"docker", "exec", "-i", "clab-lab-win", "/labcontainers-guest", "put", "10m", "600", `C:\Lab Data\file.txt`}
	if !reflect.DeepEqual(f.argv[0], want) {
		t.Fatalf("command = %#v, want %#v", f.argv[0], want)
	}
}
