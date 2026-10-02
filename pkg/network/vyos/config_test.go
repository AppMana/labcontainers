package vyos

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"
)

type fakeGuest struct {
	argv     []string
	commands []Command
	output   string
	err      error
}

func (g *fakeGuest) Pipe(_ context.Context, r io.Reader, argv ...string) ([]byte, error) {
	g.argv = argv
	if err := json.NewDecoder(r).Decode(&g.commands); err != nil {
		return nil, err
	}
	return []byte(g.output), g.err
}
func TestNativeConfigPathsRemainData(t *testing.T) {
	want := []Command{{"set", []string{"interfaces", "ethernet", "eth0", "description", "a '; $(bad)"}}}
	g := &fakeGuest{output: "LABCONTAINERS_VYOS_COMMITTED\n"}
	if err := Apply(context.Background(), g, want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, g.commands) || !reflect.DeepEqual(g.argv[:5], []string{"runuser", "-u", "vyos", "--", "/usr/bin/python3"}) {
		t.Fatal("paths interpolated or native session bypassed")
	}
}
func TestRejectUnsafeOrUnacknowledgedConfig(t *testing.T) {
	for _, cmds := range [][]Command{nil, {{"delete", []string{"interfaces"}}}, {{"exec", []string{"bad", "cmd"}}}, {{"set", []string{"interfaces", ""}}}} {
		g := &fakeGuest{}
		if Apply(context.Background(), g, cmds) == nil || len(g.argv) != 0 {
			t.Fatal("invalid request reached guest")
		}
	}
	for _, g := range []*fakeGuest{{output: "not completed"}, {output: "LABCONTAINERS_VYOS_COMMITTED", err: errors.New("failed")}} {
		if Apply(context.Background(), g, []Command{{"set", []string{"system", "host-name", "tor"}}}) == nil {
			t.Fatal("failed commit accepted")
		}
	}
}
