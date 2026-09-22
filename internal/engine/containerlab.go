package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/srl-labs/containerlab/core"
)

const SupportedContainerlabVersion = "0.79.0"

var versionPattern = regexp.MustCompile(`(?m)^\s*version:\s*([0-9]+\.[0-9]+\.[0-9]+)\s*$`)

type Containerlab struct {
	Runner Runner
	Binary string
	Sudo   bool
}

type Inspection struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

func NewContainerlab() *Containerlab {
	binary := os.Getenv("LABCONTAINERS_CONTAINERLAB")
	if binary == "" {
		binary = "containerlab"
	}
	return &Containerlab{Runner: ExecRunner{}, Binary: binary, Sudo: os.Geteuid() != 0}
}

func (c *Containerlab) argv(args ...string) []string {
	bin := c.Binary
	if bin == "" {
		bin = "containerlab"
	}
	if c.Sudo {
		return append([]string{"sudo", "-n", bin}, args...)
	}
	return append([]string{bin}, args...)
}

func (c *Containerlab) run(ctx context.Context, stdin io.Reader, args ...string) (Result, error) {
	return checked(ctx, c.Runner, stdin, c.argv(args...)...)
}

func (c *Containerlab) Doctor(ctx context.Context) error {
	bin := c.Binary
	if bin == "" {
		bin = "containerlab"
	}
	// Version discovery is read-only and must work before sudo is configured;
	// deploy and mutation commands still use the privileged argv path.
	r, err := checked(ctx, c.Runner, nil, bin, "version")
	if err != nil {
		return fmt.Errorf("run Containerlab: %w", err)
	}
	m := versionPattern.FindSubmatch(append(r.Stdout, r.Stderr...))
	if len(m) != 2 {
		return fmt.Errorf("could not parse Containerlab version")
	}
	if string(m[1]) != SupportedContainerlabVersion {
		return fmt.Errorf("Containerlab %s is installed; Labcontainers requires %s", m[1], SupportedContainerlabVersion)
	}
	return nil
}

func (c *Containerlab) Validate(ctx context.Context, topology string) error {
	_, err := c.run(ctx, nil, "validate", "--topo", topology)
	return err
}

func (c *Containerlab) Deploy(ctx context.Context, topology string) error {
	_, err := c.run(ctx, nil, "deploy", "--topo", topology, "--format", "json")
	return err
}

// Plan returns Containerlab's native core.ApplyResult JSON unchanged.
func (c *Containerlab) Plan(ctx context.Context, topology string) ([]byte, error) {
	result, err := c.run(ctx, nil, "deploy", "--topo", topology, "--dry-run", "--format", "json")
	if err != nil {
		return nil, err
	}
	var plan *core.ApplyResult
	if err := json.Unmarshal(result.Stdout, &plan); err != nil || plan == nil || !plan.DryRun {
		return nil, fmt.Errorf("Containerlab returned invalid dry-run apply-result JSON")
	}
	return result.Stdout, nil
}

// ProofIsolation verifies runtime truth after deployment instead of trusting topology intent.
func (c *Containerlab) ProofIsolation(ctx context.Context, lab string, nodes []string) error {
	for _, node := range nodes {
		name, err := c.containerID(ctx, lab, node)
		if err != nil {
			return err
		}
		r, err := checked(ctx, c.Runner, nil, "docker", "inspect", name)
		if err != nil {
			return fmt.Errorf("inspect isolation for %s: %w", node, err)
		}
		var inspections []container.InspectResponse
		if err := json.Unmarshal(r.Stdout, &inspections); err != nil || len(inspections) != 1 {
			return fmt.Errorf("inspect isolation for %s: invalid Docker inspection", node)
		}
		inspection := inspections[0]
		if inspection.ContainerJSONBase == nil || inspection.HostConfig == nil || inspection.NetworkSettings == nil {
			return fmt.Errorf("inspect isolation for %s: missing Docker network state", node)
		}
		if inspection.HostConfig.NetworkMode != "none" {
			return fmt.Errorf("node %s is not isolated: runtime network mode is %q, want none", node, inspection.HostConfig.NetworkMode)
		}
		if inspection.HostConfig.PublishAllPorts || len(inspection.HostConfig.PortBindings) != 0 {
			return fmt.Errorf("node %s is not isolated: runtime host port publishing is configured", node)
		}
		for _, bindings := range inspection.NetworkSettings.Ports {
			if len(bindings) != 0 {
				return fmt.Errorf("node %s is not isolated: runtime has published host ports", node)
			}
		}
		for networkName, network := range inspection.NetworkSettings.Networks {
			if network == nil {
				return fmt.Errorf("node %s: missing state for runtime network %q", node, networkName)
			}
			// Docker 29 reports its built-in `none` network as a bookkeeping
			// endpoint. It is isolated as long as it has no usable L2/L3 identity.
			if networkName != "none" || network.Gateway != "" || network.IPAddress != "" || network.MacAddress != "" || network.IPv6Gateway != "" || network.GlobalIPv6Address != "" {
				return fmt.Errorf("node %s is not isolated: attached runtime network %q has connectivity", node, networkName)
			}
		}
	}
	return nil
}

func (c *Containerlab) Destroy(ctx context.Context, topology string) error {
	_, destroyErr := c.run(ctx, nil, "destroy", "--topo", topology, "--cleanup")
	// Containerlab 0.79 may report a successful destroy while leaving a
	// generic_vm container that was stopped and started during the test. The
	// topology-path label is injected by Containerlab and scopes this fallback
	// to the exact session; never sweep by name or image.
	listed, listErr := c.Runner.Run(ctx, nil, "docker", "ps", "-aq", "--filter", "label=clab-topo-file="+topology)
	if listErr == nil && listed.ExitCode == 0 {
		ids := strings.Fields(string(listed.Stdout))
		if len(ids) > 0 {
			args := append([]string{"docker", "rm", "-f"}, ids...)
			removed, removeErr := c.Runner.Run(ctx, nil, args...)
			if removeErr != nil {
				return removeErr
			}
			if removed.ExitCode != 0 {
				return &CommandError{Argv: args, Result: removed}
			}
		}
	} else if destroyErr == nil {
		if listErr != nil {
			return listErr
		}
		return &CommandError{Argv: []string{"docker", "ps", "--filter", "label=clab-topo-file=" + topology}, Result: listed}
	}
	return destroyErr
}

func (c *Containerlab) Lifecycle(ctx context.Context, topology, node, action string) error {
	switch action {
	case "crash", "stop", "restart":
		if err := c.saveAttachments(ctx, topology, node); err != nil {
			return fmt.Errorf("preserve peer attachments: %w", err)
		}
	}
	if action == "crash" {
		return c.crash(ctx, topology, node)
	}
	switch action {
	case "stop", "start", "restart":
	default:
		return fmt.Errorf("unknown lifecycle action %q", action)
	}
	_, lifecycleErr := c.run(ctx, nil, action, "--topo", topology, "--node", node)
	if lifecycleErr != nil || action == "stop" {
		return lifecycleErr
	}
	// Do not turn a native start/restart into a whole-lab deployment. Missing
	// containers or links require an explicitly reviewed Plan/Apply operation.
	return c.restoreAttachments(ctx, topology, node)
}

func (c *Containerlab) RestoreAttachments(ctx context.Context, topology, node string) error {
	return c.restoreAttachments(ctx, topology, node)
}

// Replace removes one node, then lets Containerlab's convergent full deploy restore that node and
// all of its links without perturbing healthy nodes.
func (c *Containerlab) Replace(ctx context.Context, topology, node string) error {
	if err := c.saveAttachments(ctx, topology, node); err != nil {
		return err
	}
	if _, err := c.run(ctx, nil, "destroy", "--topo", topology, "--node-filter", node); err != nil {
		return err
	}
	if err := c.Deploy(ctx, topology); err != nil {
		return err
	}
	return c.restoreAttachments(ctx, topology, node)
}

func (c *Containerlab) Exec(ctx context.Context, lab, node, control string, timeout time.Duration, stdin []byte, argv []string) (Result, error) {
	if len(argv) == 0 {
		return Result{}, errors.New("argv is empty")
	}
	container, err := c.containerID(ctx, lab, node)
	if err != nil {
		return Result{}, err
	}
	args := []string{"docker", "exec"}
	if stdin != nil {
		args = append(args, "-i")
	}
	args = append(args, container)
	if control == "qga" {
		input := "0"
		if stdin != nil {
			input = "1"
		}
		args = append(args, "/labcontainers-guest", "exec", timeout.String(), input)
	}
	args = append(args, argv...)
	return c.Runner.Run(ctx, bytes.NewReader(stdin), args...)
}

func (c *Containerlab) Put(ctx context.Context, lab, node, control, path string, mode uint32, content []byte) error {
	if control == "qga" {
		container, err := c.containerID(ctx, lab, node)
		if err != nil {
			return err
		}
		r, err := c.Runner.Run(ctx, bytes.NewReader(content), "docker", "exec", "-i", container,
			"/labcontainers-guest", "put", "10m", strconv.FormatUint(uint64(mode), 8), path)
		if err != nil {
			return err
		}
		if r.ExitCode != 0 {
			return &CommandError{Argv: []string{"put", path}, Result: r}
		}
		return nil
	}
	quoted := shellQuote(path)
	command := "mkdir -p -- \"$(dirname -- " + quoted + ")\" && cat > " + quoted
	if r, err := c.Exec(ctx, lab, node, control, 2*time.Minute, content, []string{"sh", "-c", command}); err != nil {
		return err
	} else if r.ExitCode != 0 {
		return &CommandError{Argv: []string{"put", path}, Result: r}
	}
	r, err := c.Exec(ctx, lab, node, control, 2*time.Minute, nil, []string{"chmod", strconv.FormatUint(uint64(mode), 8), path})
	if err != nil {
		return err
	}
	if r.ExitCode != 0 {
		return &CommandError{Argv: []string{"chmod", path}, Result: r}
	}
	return nil
}

func (c *Containerlab) SetLink(ctx context.Context, lab, node, _ string, iface string, up bool) error {
	state := "down"
	if up {
		state = "up"
	}
	// The name belongs to the native topology's endpoint, not to the guest OS.
	// A VM's guest might name the device ens2 or Ethernet; cutting the wrapper
	// endpoint works across OSes and leaves serial QGA control available.
	r, err := c.Exec(ctx, lab, node, "container", 30*time.Second, nil, []string{"ip", "link", "set", iface, state})
	if err != nil {
		return err
	}
	if r.ExitCode != 0 {
		return &CommandError{Argv: []string{"ip", "link", "set", iface, state}, Result: r}
	}
	return nil
}

func (c *Containerlab) Netem(ctx context.Context, node, iface string, delay, jitter time.Duration, loss float64, rate uint64, corruption float64) error {
	args := []string{"tools", "netem", "set", "--node", node, "--interface", iface}
	if delay != 0 {
		args = append(args, "--delay", delay.String())
	}
	if jitter != 0 {
		args = append(args, "--jitter", jitter.String())
	}
	if loss != 0 {
		args = append(args, "--loss", fmt.Sprint(loss))
	}
	if rate != 0 {
		args = append(args, "--rate", fmt.Sprint(rate))
	}
	if corruption != 0 {
		args = append(args, "--corruption", fmt.Sprint(corruption))
	}
	_, err := c.run(ctx, nil, args...)
	return err
}

func (c *Containerlab) ResetNetem(ctx context.Context, node, iface string) error {
	_, err := c.run(ctx, nil, "tools", "netem", "reset", "--node", node, "--interface", iface)
	return err
}

func (c *Containerlab) Inspect(ctx context.Context, topology string) ([]Inspection, error) {
	r, err := c.run(ctx, nil, "inspect", "--topo", topology, "--format", "json")
	if err != nil {
		return nil, err
	}
	var grouped map[string][]Inspection
	if err := json.Unmarshal(r.Stdout, &grouped); err != nil {
		return nil, fmt.Errorf("decode Containerlab inspection: %w", err)
	}
	var out []Inspection
	for _, nodes := range grouped {
		out = append(out, nodes...)
	}
	return out, nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
