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

	if !allowExternal {
		ensureMgmtSkipped(root)
	}
	if err := validateAndLabel(topology, nodes, sessionID, allowExternal); err != nil {
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
	defaults := mappingValue(topology, "defaults")
	groups := mappingValue(topology, "groups")
	for i := 0; i < len(nodes.Content); i += 2 {
		name, node := nodes.Content[i].Value, nodes.Content[i+1]
		names = append(names, name)
		if resolvedKind(node, groups, defaults) != "bridge" {
			testNodes = append(testNodes, name)
		}
	}
	sort.Strings(names)
	sort.Strings(testNodes)
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("render private topology: %w", err)
	}
	return &Prepared{YAML: out, Name: name, Nodes: names, TestNodes: testNodes}, nil
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

func validateAndLabel(topology, nodes *yaml.Node, sessionID string, allowExternal bool) error {
	defaults := mappingValue(topology, "defaults")
	kinds := mappingValue(topology, "kinds")
	groups := mappingValue(topology, "groups")
	for i := 0; i < len(nodes.Content); i += 2 {
		name, node := nodes.Content[i].Value, nodes.Content[i+1]
		if node.Kind != yaml.MappingNode {
			return fmt.Errorf("node %q must be a mapping", name)
		}
		groupName := scalarValue(node, "group")
		group := childMapping(groups, groupName)
		kind := resolvedKind(node, groups, defaults)
		kindDef := childMapping(kinds, kind)
		mode := inheritedScalar("network-mode", node, group, kindDef, defaults)
		if !allowExternal && kind != "bridge" && strings.ToLower(mode) != "none" {
			return fmt.Errorf("node %q resolves network-mode to %q; isolated labs require network-mode: none", name, mode)
		}
		if !allowExternal {
			if len(inheritedSequence("ports", node, group, kindDef, defaults)) != 0 {
				return fmt.Errorf("node %q publishes host ports in an isolated lab", name)
			}
		}
		labels := ensureMapping(node, "labels")
		setScalar(labels, "labcontainers.appmana.com/session", sessionID)
		setScalar(labels, "labcontainers.appmana.com/managed", "true")
	}
	if !allowExternal {
		links := mappingValue(topology, "links")
		if links != nil {
			for _, link := range links.Content {
				t := scalarValue(link, "type")
				switch t {
				case "host", "macvlan", "vxlan", "vxlan-stitch":
					return fmt.Errorf("link type %q can leave the lab and requires allowExternalAccess", t)
				}
			}
		}
	}
	return nil
}

func resolvedKind(node, groups, defaults *yaml.Node) string {
	return inheritedScalar("kind", node, childMapping(groups, scalarValue(node, "group")), defaults)
}

func ensureMgmtSkipped(root *yaml.Node) {
	mgmt := ensureMapping(root, "mgmt")
	setBool(mgmt, "skip-when-unused", true)
}

func inheritedScalar(key string, levels ...*yaml.Node) string {
	for _, n := range levels {
		if v := scalarValue(n, key); v != "" {
			return v
		}
	}
	return ""
}

func inheritedSequence(key string, levels ...*yaml.Node) []string {
	for _, n := range levels {
		v := mappingValue(n, key)
		if v == nil {
			continue
		}
		var out []string
		for _, item := range v.Content {
			out = append(out, item.Value)
		}
		return out
	}
	return nil
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

func scalarValue(n *yaml.Node, key string) string {
	v := mappingValue(n, key)
	if v == nil || v.Kind != yaml.ScalarNode {
		return ""
	}
	return v.Value
}

func childMapping(parent *yaml.Node, key string) *yaml.Node {
	if key == "" {
		return nil
	}
	return mappingValue(parent, key)
}

func ensureMapping(n *yaml.Node, key string) *yaml.Node {
	if v := mappingValue(n, key); v != nil && v.Kind == yaml.MappingNode {
		return v
	}
	k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	v := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	n.Content = append(n.Content, k, v)
	return v
}

func ensureSequence(n *yaml.Node, key string) *yaml.Node {
	if v := mappingValue(n, key); v != nil && v.Kind == yaml.SequenceNode {
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
