package engine

import (
	"context"
	"io"
	"reflect"
	"testing"
)

type linkStateRunner struct {
	output string
	argv   []string
}

func (r *linkStateRunner) Run(_ context.Context, _ io.Reader, argv ...string) (Result, error) {
	if len(argv) > 1 && argv[1] == "ps" {
		return Result{Stdout: []byte("native-container-id")}, nil
	}
	r.argv = argv
	return Result{Stdout: []byte(r.output)}, nil
}

func TestLinkObservationUsesAdministrativeFlagOnWrapper(t *testing.T) {
	for _, tc := range []struct {
		output    string
		up, valid bool
	}{
		{"0x1003\n", true, true},  // UP without requiring carrier
		{"0x1042\n", false, true}, // RUNNING does not mean administratively UP
		{"0x0\n", false, true},
		{"", false, false}, {"invalid", false, false}, {"-1", false, false},
	} {
		r := &linkStateRunner{output: tc.output}
		c := &Containerlab{Runner: r}
		up, err := c.LinkUp(context.Background(), "lab", "windows", "qga", "eth1")
		if (err == nil) != tc.valid || up != tc.up {
			t.Fatalf("%s: up=%v error=%v", tc.output, up, err)
		}
		want := []string{"docker", "exec", "native-container-id", "cat", "/sys/class/net/eth1/flags"}
		if !reflect.DeepEqual(r.argv, want) {
			t.Fatalf("observed guest instead of native endpoint: %q", r.argv)
		}
	}
}
