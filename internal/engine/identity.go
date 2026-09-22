package engine

import (
	"context"
	"fmt"
	"strings"
)

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
