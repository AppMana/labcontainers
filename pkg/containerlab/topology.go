// Package containerlab connects Containerlab's native Go configuration types
// to the Labcontainers session transport. It defines no topology model.
package containerlab

import (
	"fmt"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/links"
	"gopkg.in/yaml.v2"
)

// Source serializes a native Containerlab configuration for the daemon.
// Callers construct core.Config, types.NodeDefinition and links.RawLink objects
// directly. Native struct tags determine the serialized fields.
// This does not mutate the input or add a management network. Session policy
// is applied separately by the daemon and is not bypassed by typed inputs.
func Source(config *core.Config) (*labv1.TopologySource, error) {
	if config == nil || config.Topology == nil {
		return nil, fmt.Errorf("Containerlab configuration and topology are required")
	}
	// v0.79.0's LinkDefinition.MarshalYAML emits host links as veth and
	// doesn't handle vxlan-stitch. Keep the native public types, but serialize
	// their native raw fields with the correct discriminator at this boundary.
	// No caller objects are changed and no parallel topology model is defined.
	copyConfig := *config
	copyTopology := *config.Topology
	copyTopology.Links = nil
	copyConfig.Topology = &copyTopology
	raw, err := yaml.Marshal(&copyConfig)
	if err != nil {
		return nil, fmt.Errorf("serialize Containerlab configuration: %w", err)
	}
	if len(config.Topology.Links) != 0 {
		var document map[string]any
		if err := yaml.Unmarshal(raw, &document); err != nil {
			return nil, err
		}
		topology := document["topology"].(map[any]any)
		encodedLinks := make([]map[any]any, 0, len(config.Topology.Links))
		for i, link := range config.Topology.Links {
			if link == nil || link.Link == nil {
				return nil, fmt.Errorf("link %d is nil", i)
			}
			data, err := yaml.Marshal(link.Link)
			if err != nil {
				return nil, fmt.Errorf("serialize link %d: %w", i, err)
			}
			var fields map[any]any
			if err := yaml.Unmarshal(data, &fields); err != nil {
				return nil, fmt.Errorf("serialize link %d: %w", i, err)
			}
			if fields == nil {
				return nil, fmt.Errorf("link %d has no native fields", i)
			}
			if link.Link.GetType() != links.LinkTypeBrief {
				fields["type"] = string(link.Link.GetType())
			}
			if vxlan, ok := link.Link.(*links.LinkVxlanRaw); ok {
				// Upstream's Go-only discriminator has no YAML tag. Its wire
				// representation is "type", never an additional "linktype".
				delete(fields, "linktype")
				// GetType reports "vxlan" for both forms; the native field
				// selects whether this is the stitched variant.
				if vxlan.LinkType != "" {
					fields["type"] = string(vxlan.LinkType)
				}
			}
			encodedLinks = append(encodedLinks, fields)
		}
		topology["links"] = encodedLinks
		raw, err = yaml.Marshal(document)
		if err != nil {
			return nil, fmt.Errorf("serialize Containerlab configuration: %w", err)
		}
	}
	return &labv1.TopologySource{Source: &labv1.TopologySource_Yaml{Yaml: raw}}, nil
}
