package engine

import (
	"context"
	"io"
	"reflect"
	"testing"
)

type identityRunner struct{ output string }

func (r identityRunner) Run(context.Context, io.Reader, ...string) (Result, error) {
	return Result{Stdout: []byte(r.output)}, nil
}

func TestContainerIdentityRequiresExactlyOneMatch(t *testing.T) {
	for _, output := range []string{"", "one\ntwo\n"} {
		c := &Containerlab{Runner: identityRunner{output}}
		if _, err := c.ContainerName(context.Background(), "lab", "node"); err == nil {
			t.Fatalf("accepted ambiguous or missing identity %q", output)
		}
	}
	c := &Containerlab{Runner: identityRunner{"upstream-custom-name\n"}}
	if name, err := c.ContainerName(context.Background(), "lab", "node"); err != nil || name != "upstream-custom-name" {
		t.Fatalf("name=%q error=%v", name, err)
	}
}

func TestVMFaultTargetsNativeEndpointNotGuestDevice(t *testing.T) {
	f := &fakeRunner{}
	c := &Containerlab{Runner: f}
	if err := c.SetLink(context.Background(), "lab", "win", "qga", "eth1", false); err != nil {
		t.Fatal(err)
	}
	want := []string{"docker", "exec", "native-container-id", "ip", "link", "set", "eth1", "down"}
	if !reflect.DeepEqual(f.argv[1], want) {
		t.Fatalf("fault command=%q, want %q", f.argv[1], want)
	}
}
