package spec

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	yamlv3 "gopkg.in/yaml.v3"
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

func TestPrepareRunsTestNodesUnprivilegedUnlessTopologyChooses(t *testing.T) {
	p, err := Prepare([]byte(`name: rootless
topology:
  kinds:
    linux: {cap-add: [SYS_PTRACE]}
  nodes:
    vm: {kind: generic_vm, image: vm:1, devices: [/dev/kvm]}
    peer: {kind: linux, image: alpine:3.20}
    chosen: {kind: linux, image: alpine:3.20, privileged: true}
`), "suite", "session-1", false)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Topology struct {
			Nodes map[string]struct {
				Privileged *bool    `yaml:"privileged"`
				CapAdd     []string `yaml:"cap-add"`
				Devices    []string `yaml:"devices"`
			} `yaml:"nodes"`
		} `yaml:"topology"`
	}
	if err := yamlv3.Unmarshal(p.YAML, &doc); err != nil {
		t.Fatal(err)
	}
	vm, peer, chosen := doc.Topology.Nodes["vm"], doc.Topology.Nodes["peer"], doc.Topology.Nodes["chosen"]
	if vm.Privileged == nil || *vm.Privileged || !reflect.DeepEqual(vm.CapAdd, []string{"NET_ADMIN"}) || !reflect.DeepEqual(vm.Devices, []string{"/dev/kvm", "/dev/net/tun"}) {
		t.Fatalf("vm: %+v", vm)
	}
	// Kind-level SYS_PTRACE still applies through Containerlab's merge.
	if peer.Privileged == nil || *peer.Privileged || !reflect.DeepEqual(peer.CapAdd, []string{"NET_ADMIN", "NET_RAW"}) || peer.Devices != nil {
		t.Fatalf("peer: %+v", peer)
	}
	if chosen.Privileged == nil || !*chosen.Privileged || chosen.CapAdd != nil {
		t.Fatalf("explicit privilege must be preserved: %+v", chosen)
	}
}
