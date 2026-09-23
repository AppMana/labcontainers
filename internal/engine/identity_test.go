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

func TestLabNamePreflightRejectsAnyExistingContainer(t *testing.T) {
	for _, output := range []string{"foreign-container\n", "stopped-container\n"} {
		c := &Containerlab{Runner: identityRunner{output}}
		if err := c.CheckLabNameAvailable(context.Background(), "existing"); err == nil {
			t.Fatalf("accepted an occupied lab name: %q", output)
		}
	}
	f := &fakeRunner{}
	c := &Containerlab{Runner: f}
	if err := c.CheckLabNameAvailable(context.Background(), "new"); err != nil {
		t.Fatal(err)
	}
	want := []string{"docker", "ps", "--all", "--no-trunc", "--filter", "label=containerlab=new", "--format", "{{.ID}}"}
	if !reflect.DeepEqual(f.argv[0], want) {
		t.Fatalf("preflight must include stopped and unowned containers: %q", f.argv[0])
	}
}

func TestReconciliationRejectsForeignRuntimeOwnership(t *testing.T) {
	for _, output := range []string{"container\n", "container other-session\n", "mine session\nforeign other\n"} {
		c := &Containerlab{Runner: identityRunner{output}}
		if err := c.CheckSessionOwnership(context.Background(), "lab", "session"); err == nil {
			t.Fatalf("accepted foreign runtime ownership: %q", output)
		}
	}
	c := &Containerlab{Runner: identityRunner{"container session\n"}}
	if err := c.CheckSessionOwnership(context.Background(), "lab", "session"); err != nil {
		t.Fatal(err)
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
