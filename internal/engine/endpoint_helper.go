package engine

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/srl-labs/containerlab/core"
	clabruntime "github.com/srl-labs/containerlab/runtime"
)

func validateEndpointTarget(inspection container.InspectResponse, topology, node, id string) error {
	if inspection.ContainerJSONBase == nil || inspection.State == nil || inspection.Config == nil || inspection.ID != id || !inspection.State.Running {
		return fmt.Errorf("endpoint operation requires the exact running container ID")
	}
	labels := inspection.Config.Labels
	if labels["clab-topo-file"] != topology || labels["clab-node-name"] != node || labels["labcontainers.appmana.com/managed"] != "true" {
		return fmt.Errorf("container is outside the requested managed topology/node")
	}
	return nil
}

// RunEndpointHelper performs only native parking/restoration. No deploy, Stop,
// PreStop, guest execution, disk operation or unrelated node lifecycle occurs.
// The daemon invokes its own matching executable through its existing sudo
// boundary rather than requiring an extra separately versioned helper binary.
func RunEndpointHelper(ctx context.Context, operation string, args []string) error {
	flags := flag.NewFlagSet(operation, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	topology := flags.String("topology", "", "exact local topology")
	node := flags.String("node", "", "exact node")
	id := flags.String("container-id", "", "resolved immutable runtime ID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if (operation != "park-endpoints" && operation != "restore-endpoints") || len(flags.Args()) != 0 || !filepath.IsAbs(*topology) || filepath.Clean(*topology) == "/" || strings.TrimSpace(*node) == "" || strings.TrimSpace(*id) == "" {
		return fmt.Errorf("explicit operation, absolute topology, node and container ID required")
	}
	info, err := os.Stat(*topology)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("topology must be a local regular file")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("native endpoint parking requires root")
	}
	r, err := checked(ctx, ExecRunner{}, nil, "docker", "inspect", *id)
	if err != nil {
		return err
	}
	var inspected []container.InspectResponse
	if err := json.Unmarshal(r.Stdout, &inspected); err != nil || len(inspected) != 1 {
		return fmt.Errorf("invalid runtime identity inspection")
	}
	if err := validateEndpointTarget(inspected[0], *topology, *node, *id); err != nil {
		return err
	}
	lab, err := core.NewContainerLab(core.WithTimeout(time.Minute), core.WithRuntime("docker", &clabruntime.RuntimeConfig{}), core.WithTopoPath(*topology, nil))
	if err != nil {
		return err
	}
	if err := lab.ResolveLinks(); err != nil {
		return err
	}
	target, ok := lab.Nodes[*node]
	if !ok || target.Config().LongName != strings.TrimPrefix(inspected[0].Name, "/") {
		return fmt.Errorf("native topology/runtime node identity mismatch")
	}
	if operation == "park-endpoints" {
		return target.ParkEndpoints(ctx)
	}
	return target.RestoreEndpoints(ctx)
}
