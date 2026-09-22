package spec

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparePreservesContainerlabTopologyAndAddsOwnership(t *testing.T) {
	in := []byte(`name: original
topology:
  defaults:
    network-mode: none
  nodes:
    n1:
      kind: linux
      image: alpine:3
    lan:
      kind: bridge
  links:
    - endpoints: ["n1:eth0", "lan:n1"]
`)
	p, err := Prepare(in, "suite/one", "session-1", true)
	if err != nil {
		t.Fatal(err)
	}
	got := string(p.YAML)
	for _, want := range []string{"name: suite-one", "skip-when-unused: true", "labcontainers.appmana.com/session: session-1", "endpoints:"} {
		if !strings.Contains(got, want) {
			t.Errorf("prepared topology does not contain %q:\n%s", want, got)
		}
	}
}

func TestPrepareKeepsRelativeBindMeaningAfterTopologyIsCopied(t *testing.T) {
	base := t.TempDir()
	in := []byte(`name: original
topology:
  defaults:
    network-mode: none
  nodes:
    vm:
      binds:
        - seed/vm/userdata:/userdata:ro
        - image.qcow2:/disk.qcow2:ro
`)
	prepared, err := PrepareWithOptions(in, "session", "id", false, PrepareOptions{BaseDir: base})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{filepath.Join(base, "seed/vm/userdata"), filepath.Join(base, "image.qcow2")} {
		if !strings.Contains(string(prepared.YAML), want) {
			t.Fatalf("prepared topology does not contain absolute bind %q:\n%s", want, prepared.YAML)
		}
	}
}

func TestPrepareDisablesImplicitManagementNetwork(t *testing.T) {
	p, err := Prepare([]byte(`name: safe
topology:
  nodes:
    n1:
      kind: linux
      image: alpine:3
`), "test", "id", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(p.YAML), "network-mode: none") || len(p.IsolatedNodes) != 1 {
		t.Fatalf("omitted network-mode enabled implicit connectivity: %s", p.YAML)
	}
}

func TestPrepareResolvesKindAndGroupInheritance(t *testing.T) {
	_, err := Prepare([]byte(`name: inherited
topology:
  defaults:
    network-mode: none
  kinds:
    linux:
      ports: ["22:22"]
  nodes:
    n1:
      kind: linux
`), "test", "id", false)
	if err == nil || !strings.Contains(err.Error(), "publishes host ports") {
		t.Fatalf("expected ports error, got %v", err)
	}
}

func TestPrepareAddsNodeBindsWithoutReplacingTopologyFields(t *testing.T) {
	p, err := PrepareWithOptions([]byte(`name: disks
topology:
  defaults: {network-mode: none}
  nodes:
    vm: {kind: linux, image: vm:test, binds: [existing:/existing]}
`), "test", "id", false, PrepareOptions{NodeBinds: map[string][]string{"vm": {"disk:/disk"}}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(p.YAML)
	if !strings.Contains(got, "existing:/existing") || !strings.Contains(got, "disk:/disk") {
		t.Fatalf("binds lost:\n%s", got)
	}
}
