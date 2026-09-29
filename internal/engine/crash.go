package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// crash sends SIGKILL to the wrapper's PID 1. The container runtime kills
// its remaining processes, including QEMU, without guest shutdown or flush.
// Resolve the immutable container ID through both session and node labels.
func (c *Containerlab) crash(ctx context.Context, topology, node string) error {
	listed, err := checked(ctx, c.Runner, nil, "docker", "ps", "-q", "--no-trunc", "--filter", "label=clab-topo-file="+topology, "--filter", "label=clab-node-name="+node)
	if err != nil {
		return err
	}
	ids := strings.Fields(string(listed.Stdout))
	if len(ids) != 1 {
		return fmt.Errorf("crash requires exactly one running session node, found %d", len(ids))
	}
	if err := c.endpointOperation(ctx, "park-endpoints", topology, node, ids[0]); err != nil {
		return fmt.Errorf("park endpoints before abrupt crash: %w", err)
	}
	_, err = checked(ctx, c.Runner, nil, "docker", "kill", "--signal=KILL", ids[0])
	if err != nil {
		// Restore only this node if it is still running. If the kill actually
		// completed despite an error, the helper refuses; keep parked state for
		// a later explicit Start rather than silently restarting anything.
		return errors.Join(err, c.endpointOperation(ctx, "restore-endpoints", topology, node, ids[0]))
	}
	return err
}

func (c *Containerlab) endpointOperation(ctx context.Context, operation, topology, node, id string) error {
	binary := c.EndpointHelper
	if binary == "" {
		var err error
		binary, err = os.Executable()
		if err != nil {
			return err
		}
	}
	argv := []string{binary, operation, "--topology", topology, "--node", node, "--container-id", id}
	if c.Sudo {
		argv = append([]string{"sudo", "-n"}, argv...)
	}
	_, err := checked(ctx, c.Runner, nil, argv...)
	return err
}
