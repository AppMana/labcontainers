// Package k0sctl writes the cluster document k0sctl applies. A lab that is
// installed and later changed by k0sctl goes through the same tool, phases
// and host access as a cluster an operator manages with it, which the
// node-by-node helpers in the k0s package deliberately do not model. The
// document is explicit and replaceable: applying a second document with a
// different version or configuration is how such a cluster is upgraded.
// Running k0sctl, and giving it SSH access to the hosts, is the caller's.
package k0sctl

import (
	"fmt"
	"net/netip"
	"regexp"

	native "github.com/k0sproject/k0s/pkg/apis/k0s/v1beta1"
	"sigs.k8s.io/yaml"
)

// Roles k0sctl accepts.
const (
	Controller       = "controller"
	ControllerWorker = "controller+worker"
	Worker           = "worker"
)

// Host is one machine k0sctl reaches over SSH.
type Host struct {
	Address string
	User    string
	// Port defaults to 22.
	Port int
	Role string
	// KeyPath is the private key on the machine that runs k0sctl; empty
	// leaves k0sctl's own defaults in charge.
	KeyPath      string
	InstallFlags []string
	// NoTaints lets a controller+worker run ordinary workloads.
	NoTaints bool
}

// Cluster is a k0sctl cluster document.
type Cluster struct {
	Name  string
	Hosts []Host
	// Version is the k0s version to install or move to, without a leading v.
	Version string
	// BinaryPath is a k0s binary of exactly Version on the machine that runs
	// k0sctl, uploaded to every host. Empty lets each host download it,
	// which an isolated lab cannot do.
	BinaryPath string
	// Config is the native cluster configuration, written unchanged.
	Config *native.ClusterConfig
}

var k0sVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+\+k0s\.[0-9]+$`)

// YAML serializes the document.
func (c Cluster) YAML() ([]byte, error) {
	if c.Name == "" || !k0sVersion.MatchString(c.Version) || c.Config == nil {
		return nil, fmt.Errorf("a cluster document needs a name, an exact k0s version such as 1.36.4+k0s.1, and a configuration")
	}
	if len(c.Hosts) == 0 || (c.Hosts[0].Role != Controller && c.Hosts[0].Role != ControllerWorker) {
		return nil, fmt.Errorf("the first host must be a controller")
	}
	seen := map[string]bool{}
	var hosts []any
	for _, h := range c.Hosts {
		if _, err := netip.ParseAddr(h.Address); err != nil || seen[h.Address] || h.User == "" {
			return nil, fmt.Errorf("host %q needs a unique IP address and a user", h.Address)
		}
		seen[h.Address] = true
		switch h.Role {
		case Controller, ControllerWorker, Worker:
		default:
			return nil, fmt.Errorf("host %s has unknown role %q", h.Address, h.Role)
		}
		port := h.Port
		if port == 0 {
			port = 22
		}
		ssh := map[string]any{"address": h.Address, "user": h.User, "port": port}
		if h.KeyPath != "" {
			ssh["keyPath"] = h.KeyPath
		}
		host := map[string]any{"ssh": ssh, "role": h.Role}
		if len(h.InstallFlags) != 0 {
			host["installFlags"] = h.InstallFlags
		}
		if h.NoTaints {
			if h.Role != ControllerWorker {
				return nil, fmt.Errorf("host %s: only a controller+worker carries taints to drop", h.Address)
			}
			host["noTaints"] = true
		}
		if c.BinaryPath != "" {
			host["uploadBinary"] = true
			host["k0sBinaryPath"] = c.BinaryPath
		}
		hosts = append(hosts, host)
	}
	raw, err := yaml.Marshal(c.Config)
	if err != nil {
		return nil, fmt.Errorf("encoding native k0s configuration: %w", err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	return yaml.Marshal(map[string]any{
		"apiVersion": "k0sctl.k0sproject.io/v1beta1", "kind": "Cluster",
		"metadata": map[string]any{"name": c.Name},
		"spec": map[string]any{
			"hosts": hosts,
			"k0s":   map[string]any{"version": c.Version, "dynamicConfig": false, "config": config},
		},
	})
}
