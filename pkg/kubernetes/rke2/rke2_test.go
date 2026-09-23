package rke2

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/appmana/labcontainers/pkg/rig"
)

type node struct {
	rig.Node
	argv     []string
	commands [][]string
	err      error
	failAt   int
}

func (n *node) Name() string { return "selected" }
func (n *node) Exec(_ context.Context, argv ...string) ([]byte, error) {
	n.argv = append([]string(nil), argv...)
	n.commands = append(n.commands, n.argv)
	if n.failAt == 0 || n.failAt == len(n.commands) {
		return []byte("installer diagnostic"), n.err
	}
	return nil, nil
}

func TestReadyUsesExplicitNativePaths(t *testing.T) {
	n := &node{}
	if err := Ready(context.Background(), n, "/native/kubectl", "/native/config with spaces"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"test", "-f", "/native/config with spaces"}, {"/native/kubectl", "--kubeconfig", "/native/config with spaces", "get", "--raw", "/readyz"}}
	if !reflect.DeepEqual(n.commands, want) {
		t.Fatalf("unexpected probes: %q", n.commands)
	}
	for _, step := range []int{1, 2} {
		failure := errors.New("not ready")
		n := &node{err: failure, failAt: step}
		if err := Ready(context.Background(), n, "/native/kubectl", "/native/config"); !errors.Is(err, failure) || len(n.commands) != step {
			t.Fatalf("readiness failed open: %v %v", err, n.commands)
		}
	}
	n = &node{}
	if err := Ready(context.Background(), n, "kubectl", "/config"); err == nil || len(n.commands) != 0 {
		t.Fatal("accepted implicit executable path")
	}
}

func TestNativeEnvironmentPassThrough(t *testing.T) {
	n := &node{}
	env := []string{"INSTALL_RKE2_ARTIFACT_PATH=/root/artifacts with spaces", "INSTALL_RKE2_TYPE=agent", "CUSTOM=$(touch /not-executed); value"}
	if err := Install(context.Background(), n, "/tmp/installer with spaces.sh", env...); err != nil {
		t.Fatal(err)
	}
	want := append([]string{"env", "--"}, env...)
	want = append(want, "/tmp/installer with spaces.sh")
	if !reflect.DeepEqual(n.argv, want) {
		t.Fatalf("changed native arguments: %q", n.argv)
	}
}

func TestInvalidInputDoesNotExec(t *testing.T) {
	for _, tc := range []struct {
		installer string
		env       []string
	}{
		{"relative", nil}, {"/", nil}, {"/tmp/install", []string{"--ignore-environment"}},
		{"/tmp/install", []string{"=value"}}, {"/tmp/install", []string{"-x=value"}},
	} {
		n := &node{}
		if err := Install(context.Background(), n, tc.installer, tc.env...); err == nil || n.argv != nil {
			t.Fatal("invalid input executed", tc)
		}
	}
}

func TestInstallerFailureRetainsDiagnostic(t *testing.T) {
	failure := errors.New("exit 1")
	n := &node{err: failure}
	err := Install(context.Background(), n, "/tmp/install")
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "installer diagnostic") {
		t.Fatalf("lost failure: %v", err)
	}
}
