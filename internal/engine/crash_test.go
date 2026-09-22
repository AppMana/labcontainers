package engine

import (
	"context"
	"reflect"
	"testing"
)

func TestCrashKillsOnlyResolvedTopologyNode(t *testing.T) {
	f := &fakeRunner{result: Result{Stdout: []byte("specific-container-id\n")}}
	c := &Containerlab{Runner: f}
	if err := c.crash(context.Background(), "/tmp/session/topology.yml", "vm"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"docker", "ps", "-q", "--filter", "label=clab-topo-file=/tmp/session/topology.yml", "--filter", "label=clab-node-name=vm"},
		{"docker", "kill", "--signal=KILL", "specific-container-id"},
	}
	if !reflect.DeepEqual(f.argv, want) {
		t.Fatalf("commands=%v want=%v", f.argv, want)
	}
}
