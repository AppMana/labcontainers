package containerlab_test

import (
	"reflect"
	"testing"

	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/appmana/labcontainers/pkg/spec"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/links"
	"github.com/srl-labs/containerlab/types"
	"gopkg.in/yaml.v2"
)

func TestNativeTopologyRoundTrip(t *testing.T) {
	autoRemove := false
	config := &core.Config{
		Name: "native",
		Topology: &types.Topology{
			Defaults: &types.NodeDefinition{Kind: "linux", Image: "alpine:3.20", NetworkMode: "none"},
			Nodes: map[string]*types.NodeDefinition{
				"client": {Exec: []string{"ip addr add 192.0.2.1/24 dev eth1"}, AutoRemove: &autoRemove},
				"server": {},
			},
			Links: []*links.LinkDefinition{{Link: &links.LinkVEthRaw{
				LinkCommonParams: links.LinkCommonParams{MTU: 1400, Vars: map[string]any{"test": "preserved"}},
				Endpoints: []*links.EndpointRaw{
					{Node: "client", Iface: "eth1", IPv4: "192.0.2.1/24", Vars: map[string]any{"role": "client"}},
					{Node: "server", Iface: "eth1", IPv4: "192.0.2.2/24"},
				},
			}}},
		},
	}
	source, err := clab.Source(config)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := spec.Prepare(source.GetYaml(), "session", "id", false)
	if err != nil {
		t.Fatal(err)
	}
	var got core.Config
	// Strict native decoding also catches duplicate keys introduced when the
	// daemon prepares null optional fields emitted by native Go marshaling.
	if err := yaml.UnmarshalStrict(prepared.YAML, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "session" || config.Name != "native" || config.Mgmt != nil {
		t.Fatal("preparation changed caller's configuration")
	}
	if got.Mgmt == nil || !got.Mgmt.SkipWhenUnused {
		t.Fatalf("management network not suppressed: %+v", got.Mgmt)
	}
	if got.Topology.Nodes["client"].AutoRemove == nil || *got.Topology.Nodes["client"].AutoRemove {
		t.Fatal("explicit false lost")
	}
	if !reflect.DeepEqual(config.Topology.Links, got.Topology.Links) {
		// Native unmarshal sets Type even when the input uses the native Link
		// interface alone, so compare raw links rather than that discriminator.
		if !reflect.DeepEqual(config.Topology.Links[0].Link, got.Topology.Links[0].Link) {
			t.Fatalf("native link changed: %#v", got.Topology.Links[0].Link)
		}
	}
}

func TestSourceRequiresConfiguration(t *testing.T) {
	for _, config := range []*core.Config{nil, {}} {
		if _, err := clab.Source(config); err == nil {
			t.Fatal("expected error")
		}
	}
}

func TestNativeExternalLinksRoundTrip(t *testing.T) {
	for name, raw := range map[string]links.RawLink{
		"host":         &links.LinkHostRaw{HostInterface: "test0", Endpoint: &links.EndpointRaw{Node: "vm", Iface: "eth1", IPv4: "192.0.2.1/24"}},
		"vxlan":        &links.LinkVxlanRaw{LinkType: links.LinkTypeVxlan, Remote: "192.0.2.2", VNI: 100, Endpoint: links.EndpointRaw{Node: "vm", Iface: "eth1"}},
		"vxlan-stitch": &links.LinkVxlanRaw{LinkType: links.LinkTypeVxlanStitch, Remote: "192.0.2.2", VNI: 101, Endpoint: links.EndpointRaw{Node: "vm", Iface: "eth1"}},
	} {
		t.Run(name, func(t *testing.T) {
			source, err := clab.Source(&core.Config{Name: "external", Topology: &types.Topology{
				Nodes: map[string]*types.NodeDefinition{"vm": {Kind: "generic_vm"}},
				Links: []*links.LinkDefinition{{Link: raw}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			var got core.Config
			if err := yaml.UnmarshalStrict(source.GetYaml(), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(raw, got.Topology.Links[0].Link) {
				t.Fatalf("native link changed: %#v", got.Topology.Links[0].Link)
			}
		})
	}
}

func TestSourceRejectsNilLinksWithoutPanic(t *testing.T) {
	for _, link := range []*links.LinkDefinition{nil, {}, {Link: (*links.LinkHostRaw)(nil)}} {
		_, err := clab.Source(&core.Config{Topology: &types.Topology{Links: []*links.LinkDefinition{link}}})
		if err == nil {
			t.Fatal("nil link accepted")
		}
	}
}
