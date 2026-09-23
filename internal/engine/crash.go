package engine

import (
	"context"
	"fmt"
	"strings"
)

// crash sends SIGKILL to the wrapper's PID 1. The container runtime kills
// its remaining processes, including QEMU, without guest shutdown or flush.
// Resolve the immutable container ID through both session and node labels.
func (c *Containerlab) crash(ctx context.Context, topology, node string) error {
	listed, err := checked(ctx, c.Runner, nil, "docker", "ps", "-q", "--filter", "label=clab-topo-file="+topology, "--filter", "label=clab-node-name="+node)
	if err != nil {
		return err
	}
	ids := strings.Fields(string(listed.Stdout))
	if len(ids) != 1 {
		return fmt.Errorf("crash requires exactly one running session node, found %d", len(ids))
	}
	_, err = checked(ctx, c.Runner, nil, "docker", "kill", "--signal=KILL", ids[0])
	return err
}
