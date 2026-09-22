package engine

import (
	"context"
	"reflect"
	"testing"
)

func TestPlanPassesNativeJSONThrough(t *testing.T) {
	raw := []byte(`{"dry-run":true,"node-change-reasons":{"vm":"added link"},"future-upstream-field":42}`)
	f := &fakeRunner{result: Result{Stdout: raw}}
	c := &Containerlab{Runner: f, Binary: "clab"}
	got, err := c.Plan(context.Background(), "/lab/topology.clab.yml")
	if err != nil || string(got) != string(raw) {
		t.Fatalf("result=%s error=%v", got, err)
	}
	want := []string{"clab", "deploy", "--topo", "/lab/topology.clab.yml", "--dry-run", "--format", "json"}
	if !reflect.DeepEqual(f.argv[0], want) {
		t.Fatalf("command=%q", f.argv[0])
	}
	f.result.Stdout = []byte("not JSON")
	if _, err := c.Plan(context.Background(), "/lab/topology.clab.yml"); err == nil {
		t.Fatal("accepted invalid native result")
	}
}
