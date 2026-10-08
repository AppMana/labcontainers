package engine

import (
	"context"
	"io"
	"reflect"
	"slices"
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
		// UP without requiring carrier.
		{"5: eth1@if4: <BROADCAST,MULTICAST,UP> mtu 1500 qdisc noqueue state DOWN \\    link/ether aa:c1:ab:ed:32:9e brd ff:ff:ff:ff:ff:ff\n", true, true},
		// LOWER_UP (carrier) does not mean administratively UP.
		{"5: eth1: <BROADCAST,MULTICAST,LOWER_UP> mtu 1500 qdisc noop state DOWN qlen 1000\\    link/ether aa:c1:ab:ed:32:9e brd ff:ff:ff:ff:ff:ff\n", false, true},
		{"5: eth1: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue master br0 state UP\n", true, true},
		{"6: eth2: <BROADCAST,MULTICAST,UP> mtu 1500\n", false, false}, // another interface
		{"", false, false}, {"invalid", false, false}, {"5: eth1: BROADCAST", false, false},
	} {
		r := &linkStateRunner{output: tc.output}
		c := &Containerlab{Runner: r}
		up, err := c.LinkUp(context.Background(), "lab", "windows", "qga", "eth1")
		if (err == nil) != tc.valid || up != tc.up {
			t.Fatalf("%q: up=%v error=%v", tc.output, up, err)
		}
		want := []string{"docker", "exec", "native-container-id", "ip", "-o", "link", "show", "dev", "eth1"}
		if !reflect.DeepEqual(r.argv, want) {
			t.Fatalf("observed guest instead of native endpoint: %q", r.argv)
		}
	}
}

func TestParseIPLinkReadsMaster(t *testing.T) {
	link, err := parseIPLink("4: d0: <BROADCAST,NOARP> mtu 1500 qdisc noop master br0 state DOWN qlen 1000\\    link/ether 22:6f:f0:07:2f:f6 brd ff:ff:ff:ff:ff:ff")
	if err != nil || link.name != "d0" || link.master != "br0" || slices.Contains(link.flags, "UP") {
		t.Fatalf("%+v %v", link, err)
	}
	link, err = parseIPLink("2: eth3@if7: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP")
	if err != nil || link.name != "eth3" || link.master != "" || !slices.Contains(link.flags, "UP") {
		t.Fatalf("%+v %v", link, err)
	}
}
