package engine

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
)

// The real direct Linux-VM <-> Windows-VM topology loses both veth ends when
// the wrapper is killed without parking. Start only restores parked links;
// it does not recreate links. Require native parking before SIGKILL, never a
// graceful stop or a whole-topology deploy as an implicit substitute.
func TestCrashParksEndpointsBeforeKill(t *testing.T) {
	f := &fakeRunner{result: Result{Stdout: []byte("specific-container-id\n")}}
	c := &Containerlab{Runner: f}
	if err := c.crash(context.Background(), "/tmp/session/topology.yml", "vm"); err != nil {
		t.Fatal(err)
	}
	parked := false
	for _, argv := range f.argv {
		line := strings.Join(argv, " ")
		if strings.Contains(line, "park-endpoints") {
			parked = true
		}
		if strings.Contains(line, "deploy") || strings.Contains(line, " stop ") {
			t.Fatalf("crash changed semantics: %v", argv)
		}
		if len(argv) > 1 && argv[0] == "docker" && argv[1] == "kill" && !parked {
			t.Fatal("crash destroys unparked dataplane endpoints; native Start cannot restore eth1")
		}
	}
	if !parked {
		t.Fatal("no native endpoint parking")
	}
}

func TestCrashKillsOnlyResolvedTopologyNode(t *testing.T) {
	f := &fakeRunner{result: Result{Stdout: []byte("specific-container-id\n")}}
	c := &Containerlab{Runner: f, EndpointHelper: "labd"}
	if err := c.crash(context.Background(), "/tmp/session/topology.yml", "vm"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"docker", "ps", "-q", "--no-trunc", "--filter", "label=clab-topo-file=/tmp/session/topology.yml", "--filter", "label=clab-node-name=vm"},
		{"labd", "park-endpoints", "--topology", "/tmp/session/topology.yml", "--node", "vm", "--container-id", "specific-container-id"},
		{"docker", "kill", "--signal=KILL", "specific-container-id"},
	}
	if !reflect.DeepEqual(f.argv, want) {
		t.Fatalf("commands=%v want=%v", f.argv, want)
	}
}

type crashFailureRunner struct {
	fakeRunner
	fail string
}

func (f *crashFailureRunner) Run(ctx context.Context, input io.Reader, argv ...string) (Result, error) {
	if strings.Contains(strings.Join(argv, " "), f.fail) {
		f.argv = append(f.argv, argv)
		return Result{ExitCode: 23}, nil
	}
	return f.fakeRunner.Run(ctx, input, argv...)
}

func TestCrashParkingAndKillFailures(t *testing.T) {
	for _, failure := range []string{"park-endpoints", "docker kill"} {
		t.Run(failure, func(t *testing.T) {
			f := &crashFailureRunner{fakeRunner: fakeRunner{result: Result{Stdout: []byte("exact-id\n")}}, fail: failure}
			c := &Containerlab{Runner: f, EndpointHelper: "/pinned/labd", Sudo: true}
			if err := c.crash(context.Background(), "/owned/topology.yml", "windows"); err == nil {
				t.Fatal("ignored failure")
			}
			killed, restored := false, false
			for _, argv := range f.argv {
				line := strings.Join(argv, " ")
				killed = killed || strings.HasPrefix(line, "docker kill ")
				restored = restored || strings.Contains(line, "restore-endpoints")
				if strings.Contains(line, "endpoints") && !reflect.DeepEqual(argv[:3], []string{"sudo", "-n", "/pinned/labd"}) {
					t.Fatalf("wrong privilege helper: %v", argv)
				}
			}
			if failure == "park-endpoints" && (killed || restored) {
				t.Fatal("parking failure must prevent kill")
			}
			if failure == "docker kill" && (!killed || !restored) {
				t.Fatal("failed kill must attempt scoped endpoint restoration")
			}
		})
	}
}
