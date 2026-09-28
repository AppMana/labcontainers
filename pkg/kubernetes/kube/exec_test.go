package kube

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/appmana/labcontainers/pkg/rig"
)

type workloadNode struct {
	fakeNode
	contexts []context.Context
	piped    bool
	output   []byte
	err      error
}

func (n *workloadNode) Exec(ctx context.Context, argv ...string) ([]byte, error) {
	n.contexts = append(n.contexts, ctx)
	n.calls = append(n.calls, append([]string(nil), argv...))
	if argv[len(argv)-1] == "/readyz" {
		return []byte("ok"), nil
	}
	return n.output, n.err
}

func (n *workloadNode) Pipe(ctx context.Context, input io.Reader, argv ...string) ([]byte, error) {
	n.piped = true
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, err
	}
	n.stdin = string(data)
	return n.Exec(ctx, argv...)
}

func TestExecPodPreservesCommandStdinContextAndFailure(t *testing.T) {
	for _, withInput := range []bool{false, true} {
		for _, prefix := range [][]string{nil, {"/usr/local/bin/k0s", "kubectl"}} {
			n := &workloadNode{output: []byte("workload output\x00"), err: &rig.ExitError{Node: "controller", Code: 23, Stderr: []byte("workload failed")}}
			c := &Client{Bastion: n, ControlPlanes: []string{"192.0.2.10", "192.0.2.11"}, Kubectl: prefix}
			ctx, cancel := context.WithCancel(context.Background())
			var input io.Reader
			if withInput {
				input = strings.NewReader("stdin\x00bytes\n")
			}
			args := []string{"program", "a b", `C:\path\file`, "; echo should-not-run", "--literal"}
			out, err := c.ExecPod(ctx, "lab", "windows-workload", "client", input, args...)
			cancel()
			if !errors.Is(err, n.err) || string(out) != string(n.output) {
				t.Fatalf("lost workload output/error: %q %v", out, err)
			}
			if len(n.calls) != 2 {
				t.Fatalf("workload retried after successful readyz: %v", n.calls)
			}
			command := append([]string(nil), prefix...)
			if len(command) == 0 {
				command = []string{"kubectl"}
			}
			wantProbe := append(append([]string(nil), command...), "--server=https://192.0.2.10:6443", "--request-timeout=2s", "get", "--raw", "/readyz")
			wantExec := append(append([]string(nil), command...), "--server=https://192.0.2.10:6443", "exec", "--namespace=lab", "windows-workload", "--container=client")
			if withInput {
				wantExec = append(wantExec, "-i")
			}
			wantExec = append(wantExec, "--")
			wantExec = append(wantExec, args...)
			if !reflect.DeepEqual(n.calls[0], wantProbe) || !reflect.DeepEqual(n.calls[1], wantExec) {
				t.Fatalf("argv changed: %v", n.calls)
			}
			if n.piped != withInput || (withInput && n.stdin != "stdin\x00bytes\n") {
				t.Fatalf("stdin changed: piped=%v bytes=%q", n.piped, n.stdin)
			}
			for _, got := range n.contexts {
				if got != ctx {
					t.Fatal("caller context replaced")
				}
			}
		}
	}
}

func TestConfiguredKubectlPrefixAppliesToRunAndApply(t *testing.T) {
	n := &fakeNode{}
	backing := []string{"/usr/local/bin/k0s", "kubectl", "sentinel", "sentinel", "sentinel"}
	c := &Client{Bastion: n, ControlPlanes: []string{"192.0.2.10"}, Kubectl: backing[:2]}
	if _, err := c.Run(context.Background(), "get", "nodes"); err != nil {
		t.Fatal(err)
	}
	if err := c.Apply(context.Background(), []byte("manifest bytes")); err != nil {
		t.Fatal(err)
	}
	for _, argv := range n.calls {
		if len(argv) < 3 || !reflect.DeepEqual(argv[:2], backing[:2]) {
			t.Fatalf("prefix lost: %v", argv)
		}
	}
	if n.stdin != "manifest bytes" || backing[2] != "sentinel" {
		t.Fatalf("mutated stdin or prefix backing: %q %v", n.stdin, backing)
	}
}

func TestExecPodRequiresExplicitTargetAndCommand(t *testing.T) {
	for _, tc := range []struct {
		namespace, pod, container string
		args                      []string
	}{
		{"", "pod", "container", []string{"true"}}, {"ns", "", "container", []string{"true"}},
		{"ns", "pod", "", []string{"true"}}, {"ns", "pod", "container", nil}, {"ns", "--all", "container", []string{"true"}},
	} {
		n := &fakeNode{}
		c := &Client{Bastion: n, ControlPlanes: []string{"192.0.2.10"}}
		if _, err := c.ExecPod(context.Background(), tc.namespace, tc.pod, tc.container, nil, tc.args...); err == nil {
			t.Errorf("accepted incomplete/ambiguous exec: %+v", tc)
		}
		if len(n.calls) != 0 {
			t.Errorf("invalid exec contacted cluster: %v", n.calls)
		}
	}
}

type forbiddenInput struct{ t *testing.T }

func (r forbiddenInput) Read([]byte) (int, error) {
	r.t.Error("stdin consumed without a ready API member")
	return 0, io.EOF
}

func TestExecPodNeverRunsWithoutAuthenticatedReadyMember(t *testing.T) {
	n := &fakeNode{fail: func([]string) bool { return true }}
	c := &Client{Bastion: n, ControlPlanes: []string{"192.0.2.10", "192.0.2.11"}, Kubectl: []string{"/usr/local/bin/k0s", "kubectl"}}
	if _, err := c.ExecPod(context.Background(), "lab", "pod", "client", forbiddenInput{t}, "workload"); err == nil {
		t.Fatal("ran workload without ready member")
	}
	if len(n.calls) != 2 {
		t.Fatalf("unexpected calls: %v", n.calls)
	}
	for _, argv := range n.calls {
		if argv[len(argv)-1] != "/readyz" {
			t.Fatalf("non-readiness command: %v", argv)
		}
	}
}
