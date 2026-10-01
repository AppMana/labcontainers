package containerd

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/appmana/labcontainers/pkg/rig"
)

type imageNode struct {
	rig.Commands
	commands [][]string
	err      error
	ready    []byte
}

func (n *imageNode) Name() string { return "windows" }
func (n *imageNode) Exec(_ context.Context, argv ...string) ([]byte, error) {
	n.commands = append(n.commands, append([]string(nil), argv...))
	return n.ready, n.err
}

func TestOfflineImagesUseExplicitNativeRuntime(t *testing.T) {
	n := &imageNode{ready: []byte("repo/image@sha256:abc\r\n")}
	p := Images{Command: []string{`C:\Program Files\containerd\ctr.exe`, "--namespace", "k8s.io"}, Snapshotter: "windows"}
	if err := p.Import(context.Background(), n, []string{`D:\image one.tar`}, true); err != nil {
		t.Fatal(err)
	}
	if err := p.RequireReady(context.Background(), n, []string{"repo/image@sha256:abc"}); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{p.Command[0], "--namespace", "k8s.io", "images", "import", "--all-platforms", "--no-unpack", "--snapshotter", "windows", `D:\image one.tar`},
		{p.Command[0], "--namespace", "k8s.io", "images", "check", "--snapshotter", "windows", "--quiet"},
	}
	if !reflect.DeepEqual(n.commands, want) {
		t.Fatalf("native argv changed: %v", n.commands)
	}
	if err := p.RequireReady(context.Background(), n, []string{"repo/image@sha256:ab"}); err == nil {
		t.Fatal("accepted partial image identity")
	}
}

func TestOfflineImagesRejectInvalidInputBeforeExecution(t *testing.T) {
	for _, p := range []Images{{}, {Command: []string{"ctr"}}, {Command: []string{""}, Snapshotter: "windows"}} {
		n := &imageNode{}
		if err := p.Import(context.Background(), n, []string{"x.tar"}, false); err == nil || len(n.commands) != 0 {
			t.Fatal("invalid runtime touched guest")
		}
	}
	p := Images{Command: []string{"k0s", "ctr"}, Snapshotter: "windows"}
	for _, files := range [][]string{nil, {"first.tar", ""}, {"--help"}} {
		n := &imageNode{}
		if err := p.Import(context.Background(), n, files, false); err == nil || len(n.commands) != 0 {
			t.Fatal("invalid archive touched guest")
		}
	}
	n := &imageNode{}
	if err := p.RequireReady(context.Background(), n, nil); err == nil || len(n.commands) != 0 {
		t.Fatal("empty readiness check accepted")
	}
}

func TestOfflineImagesStopOnNativeFailure(t *testing.T) {
	boom := errors.New("native image failure")
	n := &imageNode{err: boom}
	p := Images{Command: []string{"k0s", "ctr"}, Snapshotter: "windows"}
	if err := p.Import(context.Background(), n, []string{"one.tar", "two.tar"}, false); !errors.Is(err, boom) || len(n.commands) != 1 {
		t.Fatalf("lost failure: %v", err)
	}
}

func TestLocalUnpackUsesDistributionWrapperWithoutShell(t *testing.T) {
	n := &imageNode{}
	p := Images{Command: []string{`C:\k0s.exe`, "ctr"}, Snapshotter: "windows", LocalImport: true}
	if err := p.Import(context.Background(), n, []string{`D:\workload.tar`}, false); err != nil {
		t.Fatal(err)
	}
	want := []string{`C:\k0s.exe`, "ctr", "images", "import", "--all-platforms", "--local", "--snapshotter", "windows", `D:\workload.tar`}
	if !reflect.DeepEqual(n.commands[0], want) {
		t.Fatalf("wrong local unpack: %v", n.commands)
	}
	if err := p.RequireReady(context.Background(), n, []string{"missing"}); err == nil {
		t.Fatal("missing image passed")
	}
	n.err = errors.New("check failed")
	if err := p.RequireReady(context.Background(), n, []string{"missing"}); !errors.Is(err, n.err) {
		t.Fatalf("lost check failure: %v", err)
	}
}
