package engine

import (
	"context"
	"fmt"
	"strings"
)

// CheckLabNameAvailable rejects existing runtime containers even when they
// belong to another daemon's state directory or a plain Containerlab CLI lab.
// This is a preflight, not an inter-process reservation of the name.
func (c *Containerlab) CheckLabNameAvailable(ctx context.Context, lab string) error {
	result, err := checked(ctx, c.Runner, nil, "docker", "ps", "--all", "--no-trunc",
		"--filter", "label=containerlab="+lab, "--format", "{{.ID}}")
	if err != nil {
		return err
	}
	if len(strings.Fields(string(result.Stdout))) != 0 {
		return fmt.Errorf("lab name %q already has runtime containers; refusing to reconcile another lab", lab)
	}
	return nil
}

// ContainerName resolves runtime truth instead of assuming Containerlab's
// default naming prefix. Netem needs the name for native stitched-link lookup.
func (c *Containerlab) ContainerName(ctx context.Context, lab, node string) (string, error) {
	return c.containerIdentity(ctx, lab, node, "{{.Names}}")
}

func (c *Containerlab) containerID(ctx context.Context, lab, node string) (string, error) {
	return c.containerIdentity(ctx, lab, node, "{{.ID}}")
}

func (c *Containerlab) containerIdentity(ctx context.Context, lab, node, format string) (string, error) {
	result, err := checked(ctx, c.Runner, nil, "docker", "ps", "--no-trunc",
		"--filter", "label=containerlab="+lab,
		"--filter", "label=clab-node-name="+node,
		"--filter", "label=labcontainers.appmana.com/managed=true",
		"--format", format)
	if err != nil {
		return "", err
	}
	values := strings.Fields(string(result.Stdout))
	if len(values) != 1 {
		return "", fmt.Errorf("node %q in lab %q resolves to %d running containers; expected exactly one", node, lab, len(values))
	}
	return values[0], nil
}
