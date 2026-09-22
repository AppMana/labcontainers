// Package k0s couples k0s's native configuration and CLI to an existing lab
// node. It creates no nodes, networks, CNI, routes, or artifact downloads.
package k0s

import (
	"bytes"
	"context"
	"fmt"
	"path"

	"github.com/appmana/labcontainers/pkg/rig"
	native "github.com/k0sproject/k0s/pkg/apis/k0s/v1beta1"
	"sigs.k8s.io/yaml"
)

// WriteConfig writes the caller's upstream object without invoking k0s's
// default constructors or changing it. The path is explicit, as is the
// corresponding --config argument passed to Install. Serialization belongs at
// this boundary, not in a test's source code.
func WriteConfig(ctx context.Context, node rig.Node, filename string, config *native.ClusterConfig) error {
	if config == nil {
		return fmt.Errorf("k0s configuration is required")
	}
	if !path.IsAbs(filename) || path.Clean(filename) == "/" {
		return fmt.Errorf("k0s configuration path must be an absolute file path")
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("encoding native k0s configuration: %w", err)
	}
	if _, err := node.Exec(ctx, "mkdir", "-p", path.Dir(filename)); err != nil {
		return err
	}
	return node.Put(ctx, bytes.NewReader(data), filename, 0o600)
}

// Install invokes the already staged k0s binary's native installation command.
// Arguments are passed individually and unchanged, never through a shell. This
// does not start k0s, silently reuse an existing installation, or fetch a binary.
// Use node.Exec(ctx, "k0s", "start") when the scenario is ready to start it.
func Install(ctx context.Context, node rig.Node, args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("k0s install arguments are required")
	}
	argv := append([]string{"k0s", "install"}, args...)
	out, err := node.Exec(ctx, argv...)
	if err != nil {
		return fmt.Errorf("%s: k0s install: %w: %s", node.Name(), err, out)
	}
	return nil
}

// Ready probes the supervisor, authenticated API, and admin kubeconfig through
// the supplied node. It performs one attempt; the caller owns retry/deadline
// policy. A successful probe is not evidence of working pod networking.
func Ready(ctx context.Context, node rig.Node) error {
	for _, args := range [][]string{
		{"k0s", "status"},
		{"k0s", "kubectl", "get", "--raw", "/readyz"},
		{"k0s", "kubeconfig", "admin"},
	} {
		if _, err := node.Exec(ctx, args...); err != nil {
			return fmt.Errorf("%s: %v: %w", node.Name(), args, err)
		}
	}
	return nil
}
