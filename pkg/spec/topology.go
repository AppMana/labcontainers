// Package spec validates and prepares Containerlab topologies for isolated test sessions.
package spec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/srl-labs/containerlab/links"
	"github.com/srl-labs/containerlab/types"
	yamlv2 "gopkg.in/yaml.v2"
	"gopkg.in/yaml.v3"
)

const (
	APIVersion = "labcontainers.appmana.com/v1alpha1"
	Kind       = "Lab"
)

var safeName = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

// LabSpec extends a Containerlab topology without duplicating its node or link schema.
type LabSpec struct {
	APIVersion string                   `yaml:"apiVersion,omitempty" json:"apiVersion,omitempty"`
	Kind       string                   `yaml:"kind,omitempty" json:"kind,omitempty"`
	Topology   TopologySource           `yaml:"topology" json:"topology"`
	Nodes      map[string]NodeExtension `yaml:"nodes,omitempty" json:"nodes,omitempty"`
	Policy     Policy                   `yaml:"policy,omitempty" json:"policy,omitempty"`
	Artifacts  Artifacts                `yaml:"artifacts,omitempty" json:"artifacts,omitempty"`
}

type TopologySource struct {
	Path   string `yaml:"path,omitempty" json:"path,omitempty"`
	Inline string `yaml:"inline,omitempty" json:"inline,omitempty"`
}

type NodeExtension struct {
	Control   string        `yaml:"control,omitempty" json:"control,omitempty"`
	Bootstrap BootstrapData `yaml:"bootstrap,omitempty" json:"bootstrap,omitempty"`
	Disks     []Disk        `yaml:"disks,omitempty" json:"disks,omitempty"`
}

type BootstrapData struct {
	Format string `yaml:"format,omitempty" json:"format,omitempty"`
	Value  []byte `yaml:"value,omitempty" json:"value,omitempty"`
}

type Disk struct {
	Name       string `yaml:"name" json:"name"`
	SizeBytes  uint64 `yaml:"sizeBytes" json:"sizeBytes"`
	Filesystem string `yaml:"filesystem,omitempty" json:"filesystem,omitempty"`
	MountPoint string `yaml:"mountPoint,omitempty" json:"mountPoint,omitempty"`
}

type Policy struct {
	AllowExternalAccess bool `yaml:"allowExternalAccess,omitempty" json:"allowExternalAccess,omitempty"`
}

type Artifacts struct {
	Directory string `yaml:"directory,omitempty" json:"directory,omitempty"`
}

// Prepared is the private, session-named Containerlab topology and its node metadata.
type Prepared struct {
	YAML      []byte
	Name      string
	Nodes     []string
	TestNodes []string
	// IsolatedNodes retain runtime isolation checks even when the topology
	// deliberately connects other nodes to an external network.
	IsolatedNodes []string
}

type PrepareOptions struct {
	NodeBinds map[string][]string
	// BaseDir is the source topology directory. Relative host-side bind
	// paths are made absolute before the private topology is moved into the
	// session state directory.
	BaseDir string
}

func AddNodeBind(input []byte, nodeName, bind string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(input, &doc); err != nil {
		return nil, err
	}
	root, err := mappingRoot(&doc)
	if err != nil {
		return nil, err
	}
	topology := mappingValue(root, "topology")
	nodes := mappingValue(topology, "nodes")
	node := mappingValue(nodes, nodeName)
	if node == nil {
		return nil, fmt.Errorf("unknown topology node %q", nodeName)
	}
	sequence := ensureSequence(node, "binds")
	for _, existing := range sequence.Content {
		if existing.Value == bind {
			return yaml.Marshal(&doc)
		}
	}
	sequence.Content = append(sequence.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: bind})
	return yaml.Marshal(&doc)
}

// ReadTopology resolves exactly one topology source.
func ReadTopology(src TopologySource, baseDir string) ([]byte, error) {
	if (src.Path == "") == (src.Inline == "") {
		return nil, errors.New("exactly one of topology.path or topology.inline is required")
	}
	if src.Inline != "" {
		return []byte(src.Inline), nil
	}
	p := src.Path
	if !filepath.IsAbs(p) {
		p = filepath.Join(baseDir, p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read topology: %w", err)
	}
	return b, nil
}

// Prepare renames a topology for a unique session, attaches ownership labels, and validates
// that the default isolation policy cannot be bypassed by inheritance.
func Prepare(input []byte, sessionName, sessionID string, allowExternal bool) (*Prepared, error) {
	return PrepareWithOptions(input, sessionName, sessionID, allowExternal, PrepareOptions{})
}

func PrepareWithOptions(input []byte, sessionName, sessionID string, allowExternal bool, options PrepareOptions) (*Prepared, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(input, &doc); err != nil {
		return nil, fmt.Errorf("parse Containerlab topology: %w", err)
	}
	root, err := mappingRoot(&doc)
	if err != nil {
		return nil, err
	}
	name := sanitize(sessionName)
	if name == "" {
		return nil, errors.New("session name is empty after sanitization")
	}
	setScalar(root, "name", name)

	topology := mappingValue(root, "topology")
	if topology == nil || topology.Kind != yaml.MappingNode {
		return nil, errors.New("topology must contain a mapping named topology")
	}
	nodes := mappingValue(topology, "nodes")
	if nodes == nil || nodes.Kind != yaml.MappingNode || len(nodes.Content) == 0 {
		return nil, errors.New("topology must define at least one node")
	}

	if mgmt := mappingValue(root, "mgmt"); mgmt != nil && mgmt.Kind != yaml.MappingNode && mgmt.Tag != "!!null" {
		return nil, errors.New("mgmt must be a mapping")
	}
	ensureMgmtSkipped(root)
	// Resolve inheritance using Containerlab itself, not a competing copy of
	// its precedence rules. Retain the syntax tree for lossless pass-through.
	rawTopology, err := yaml.Marshal(topology)
	if err != nil {
		return nil, err
	}
	native := types.NewTopology()
	if err := yamlv2.Unmarshal(rawTopology, native); err != nil {
		return nil, fmt.Errorf("decode native Containerlab topology: %w", err)
	}
	if native.Defaults == nil {
		native.Defaults = &types.NodeDefinition{}
	}
	for scope, definitions := range map[string]map[string]*types.NodeDefinition{"kinds": native.Kinds, "groups": native.Groups} {
		for name, definition := range definitions {
			if definition == nil {
				return nil, fmt.Errorf("%s.%s must be a mapping", scope, name)
			}
		}
	}
	if err := validateAndLabel(nodes, native, sessionID, allowExternal); err != nil {
		return nil, err
	}
	if options.BaseDir != "" {
		for _, parent := range []*yaml.Node{mappingValue(topology, "defaults"), mappingValue(topology, "groups"), mappingValue(topology, "kinds"), nodes} {
			if err := absolutizeBinds(parent, options.BaseDir); err != nil {
				return nil, err
			}
		}
	}
	for nodeName, binds := range options.NodeBinds {
		node := mappingValue(nodes, nodeName)
		if node == nil {
			return nil, fmt.Errorf("node extension refers to unknown topology node %q", nodeName)
		}
		sequence := ensureSequence(node, "binds")
		for _, bind := range binds {
			sequence.Content = append(sequence.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: bind})
		}
	}

	names := make([]string, 0, len(nodes.Content)/2)
	testNodes := make([]string, 0, len(nodes.Content)/2)
	isolatedNodes := make([]string, 0, len(nodes.Content)/2)
	for i := 0; i < len(nodes.Content); i += 2 {
		name := nodes.Content[i].Value
		names = append(names, name)
		if kind := native.GetNodeKind(name); kind != "bridge" && kind != "ovs-bridge" {
			testNodes = append(testNodes, name)
			if native.GetNodeNetworkMode(name) == "none" {
				isolatedNodes = append(isolatedNodes, name)
			}
		}
	}
	sort.Strings(names)
	sort.Strings(testNodes)
	sort.Strings(isolatedNodes)
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("render private topology: %w", err)
	}
	return &Prepared{YAML: out, Name: name, Nodes: names, TestNodes: testNodes, IsolatedNodes: isolatedNodes}, nil
}

func absolutizeBinds(parent *yaml.Node, baseDir string) error {
	if parent == nil || parent.Kind != yaml.MappingNode {
		return nil
	}
	// defaults is itself a node definition; groups, kinds, and nodes contain
	// named definitions. Handling both shapes keeps this helper schema-light.
	definitions := []*yaml.Node{parent}
	for i := 1; i < len(parent.Content); i += 2 {
		if parent.Content[i].Kind == yaml.MappingNode {
			definitions = append(definitions, parent.Content[i])
		}
	}
	for _, definition := range definitions {
		binds := mappingValue(definition, "binds")
		if binds == nil {
			continue
		}
		if binds.Kind != yaml.SequenceNode {
			return fmt.Errorf("binds must be a sequence")
		}
		for _, bind := range binds.Content {
			host, rest, ok := strings.Cut(bind.Value, ":")
			if !ok || host == "" || filepath.IsAbs(host) || strings.HasPrefix(host, "$") {
				continue
			}
			bind.Value = filepath.Join(baseDir, host) + ":" + rest
		}
	}
	return nil
}

func validateAndLabel(nodes *yaml.Node, native *types.Topology, sessionID string, allowExternal bool) error {
	for i := 0; i < len(nodes.Content); i += 2 {
		name, node := nodes.Content[i].Value, nodes.Content[i+1]
		if node.Tag == "!!null" {
			*node = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		if node.Kind != yaml.MappingNode {
			return fmt.Errorf("node %q must be a mapping", name)
		}
		kind := native.GetNodeKind(name)
		if !allowExternal && (kind == "bridge" || kind == "ovs-bridge") {
			return fmt.Errorf("node %q borrows a host %s and requires allowExternalAccess", name, kind)
		}
		mode := native.GetNodeNetworkMode(name)
		if kind != "bridge" && kind != "ovs-bridge" && mode == "" {
			// Omission must not turn on Docker's implicit management NIC.
			setScalar(node, "network-mode", "none")
			if native.Nodes[name] == nil {
				native.Nodes[name] = &types.NodeDefinition{}
			}
			native.Nodes[name].NetworkMode = "none"
			mode = "none"
		}
		if !allowExternal && kind != "bridge" && kind != "ovs-bridge" && mode != "none" {
			return fmt.Errorf("node %q resolves network-mode to %q; isolated labs require network-mode: none", name, mode)
		}
		if !allowExternal {
			_, ports, err := native.GetNodePorts(name)
			if err != nil {
				return fmt.Errorf("node %q ports: %w", name, err)
			}
			if len(ports) != 0 {
				return fmt.Errorf("node %q publishes host ports in an isolated lab", name)
			}
		}
		labels := ensureMapping(node, "labels")
		setScalar(labels, "labcontainers.appmana.com/session", sessionID)
		setScalar(labels, "labcontainers.appmana.com/managed", "true")
	}
	if !allowExternal {
		for _, link := range native.Links {
			if link == nil || link.Link == nil {
				return errors.New("link must not be null")
			}
			switch t := link.Link.GetType(); t {
			case links.LinkTypeHost, links.LinkTypeMgmtNet, links.LinkTypeMacVLan, links.LinkTypeVxlan, links.LinkTypeVxlanStitch:
				return fmt.Errorf("link type %q can leave the lab and requires allowExternalAccess", t)
			}
			// Extended veth endpoints can also refer directly to the host or
			// management namespace, without an external link discriminator.
			var endpoints []*links.EndpointRaw
			switch raw := link.Link.(type) {
			case *links.LinkVEthRaw:
				endpoints = raw.Endpoints
			case *links.LinkVEthStitchedRaw:
				endpoints = raw.Endpoints
			}
			for _, endpoint := range endpoints {
				if endpoint != nil && (endpoint.Node == "host" || endpoint.Node == "mgmt-net") {
					return fmt.Errorf("endpoint %q can leave the lab and requires allowExternalAccess", endpoint.Node)
				}
			}
		}
	}
	return nil
}

func ensureMgmtSkipped(root *yaml.Node) {
	mgmt := ensureMapping(root, "mgmt")
	setBool(mgmt, "skip-when-unused", true)
}

func mappingRoot(doc *yaml.Node) (*yaml.Node, error) {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("topology document must have a mapping root")
	}
	return doc.Content[0], nil
}

func mappingValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func ensureMapping(n *yaml.Node, key string) *yaml.Node {
	if v := mappingValue(n, key); v != nil {
		if v.Kind != yaml.MappingNode {
			*v = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		return v
	}
	k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	v := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	n.Content = append(n.Content, k, v)
	return v
}

func ensureSequence(n *yaml.Node, key string) *yaml.Node {
	if v := mappingValue(n, key); v != nil {
		if v.Kind != yaml.SequenceNode {
			*v = yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		}
		return v
	}
	k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	v := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	n.Content = append(n.Content, k, v)
	return v
}

func setScalar(n *yaml.Node, key, value string) {
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			n.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
			return
		}
	}
	n.Content = append(n.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

func setBool(n *yaml.Node, key string, value bool) {
	v := "false"
	if value {
		v = "true"
	}
	setScalar(n, key, v)
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			n.Content[i+1].Tag = "!!bool"
		}
	}
}

func sanitize(s string) string {
	s = strings.Trim(safeName.ReplaceAllString(s, "-"), "-._")
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}
