package spec_test

import (
	"reflect"
	"strings"
	"testing"

	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/appmana/labcontainers/pkg/spec"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/links"
	"github.com/srl-labs/containerlab/types"
	"gopkg.in/yaml.v2"
)

func TestExternalBriefLinksRequireOptIn(t *testing.T) {
	for _, endpoint := range []string{"host:test0", "mgmt-net:test0", "macvlan:eth0"} {
		t.Run(endpoint, func(t *testing.T) {
			config := &core.Config{Name: "external", Topology: &types.Topology{
				Nodes: map[string]*types.NodeDefinition{"node": {Kind: "linux"}},
				Links: []*links.LinkDefinition{{Link: &links.LinkBriefRaw{Endpoints: []string{"node:eth1", endpoint}}}},
			}}
			source, err := clab.Source(config)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = spec.Prepare(source.GetYaml(), "test", "id", false); err == nil || !strings.Contains(err.Error(), "requires allowExternalAccess") {
				t.Fatalf("external link accepted without opt-in: %v", err)
			}
			prepared, err := spec.Prepare(source.GetYaml(), "test", "id", true)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(prepared.IsolatedNodes, []string{"node"}) {
				t.Fatalf("external attachment disabled runtime network checks: %v", prepared.IsolatedNodes)
			}
		})
	}
}

func TestNativeInheritanceCannotHideConnectivity(t *testing.T) {
	for name, topology := range map[string]*types.Topology{
		"borrowed host bridge": {
			Nodes: map[string]*types.NodeDefinition{"lan": {Kind: "bridge"}},
		},
		"empty ports inherit": {
			Defaults: &types.NodeDefinition{Ports: []string{"8022:22"}},
			Nodes:    map[string]*types.NodeDefinition{"node": {Kind: "linux", Ports: []string{}}},
		},
		"default group network": {
			Defaults: &types.NodeDefinition{Group: "wan"},
			Groups:   map[string]*types.NodeDefinition{"wan": {NetworkMode: "host"}},
			Nodes:    map[string]*types.NodeDefinition{"node": {Kind: "linux"}},
		},
		"kind group network": {
			Kinds:  map[string]*types.NodeDefinition{"linux": {Group: "wan"}},
			Groups: map[string]*types.NodeDefinition{"wan": {NetworkMode: "host"}},
			Nodes:  map[string]*types.NodeDefinition{"node": {Kind: "linux"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			source, err := clab.Source(&core.Config{Name: "unsafe", Topology: topology})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := spec.Prepare(source.GetYaml(), "test", "id", false); err == nil {
				t.Fatal("inherited connectivity accepted without opt-in")
			}
		})
	}
}

func TestExternalOptInDoesNotEnableImplicitNetworks(t *testing.T) {
	source, err := clab.Source(&core.Config{Name: "mixed", Topology: &types.Topology{
		Nodes: map[string]*types.NodeDefinition{
			"isolated": {Kind: "linux"},
			"wan":      {Kind: "linux", NetworkMode: "host"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := spec.Prepare(source.GetYaml(), "test", "id", true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prepared.IsolatedNodes, []string{"isolated"}) {
		t.Fatalf("checks should cover isolated node: %v", prepared.IsolatedNodes)
	}
	var got core.Config
	if err := yaml.UnmarshalStrict(prepared.YAML, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Mgmt.SkipWhenUnused || got.Topology.Nodes["isolated"].NetworkMode != "none" {
		t.Fatal("implicit management network enabled by opt-in")
	}
	if got.Topology.Nodes["wan"].NetworkMode != "host" {
		t.Fatal("explicit external configuration lost")
	}
}
