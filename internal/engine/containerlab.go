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

type runtimeInspection struct {
	HostConfig struct {
		NetworkMode string `json:"NetworkMode"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Networks map[string]struct {
			Gateway           string `json:"Gateway"`
			IPAddress         string `json:"IPAddress"`
			MacAddress        string `json:"MacAddress"`
			IPv6Gateway       string `json:"IPv6Gateway"`
			GlobalIPv6Address string `json:"GlobalIPv6Address"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

func NewContainerlab() *Containerlab {
	return &Containerlab{Runner: ExecRunner{}, Binary: "containerlab", Sudo: os.Geteuid() != 0}
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

// ProofIsolation verifies runtime truth after deployment instead of trusting topology intent.
func (c *Containerlab) ProofIsolation(ctx context.Context, lab string, nodes []string) error {
	for _, node := range nodes {
		name := "clab-" + lab + "-" + node
		r, err := checked(ctx, c.Runner, nil, "docker", "inspect", name)
		if err != nil {
			return fmt.Errorf("inspect isolation for %s: %w", node, err)
		}
		var inspections []runtimeInspection
		if err := json.Unmarshal(r.Stdout, &inspections); err != nil || len(inspections) != 1 {
			return fmt.Errorf("inspect isolation for %s: invalid Docker inspection", node)
		}
		inspection := inspections[0]
		if inspection.HostConfig.NetworkMode != "none" {
			return fmt.Errorf("node %s is not isolated: runtime network mode is %q, want none", node, inspection.HostConfig.NetworkMode)
		}
		for networkName, network := range inspection.NetworkSettings.Networks {
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
	_, err := c.run(ctx, nil, "destroy", "--topo", topology, "--cleanup")
	return err
}

func (c *Containerlab) Lifecycle(ctx context.Context, topology, node, action string) error {
	switch action {
	case "stop", "start", "restart":
	default:
		return fmt.Errorf("unknown lifecycle action %q", action)
	}
	_, err := c.run(ctx, nil, action, "--topo", topology, "--node", node)
	return err
}

// Replace removes one node, then lets Containerlab's convergent full deploy restore that node and
// all of its links without perturbing healthy nodes.
func (c *Containerlab) Replace(ctx context.Context, topology, node string) error {
	if _, err := c.run(ctx, nil, "destroy", "--topo", topology, "--node-filter", node); err != nil {
		return err
	}
	return c.Deploy(ctx, topology)
}

func (c *Containerlab) Exec(ctx context.Context, lab, node, control string, timeout time.Duration, stdin []byte, argv []string) (Result, error) {
	if len(argv) == 0 {
		return Result{}, errors.New("argv is empty")
	}
	container := "clab-" + lab + "-" + node
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

func (c *Containerlab) SetLink(ctx context.Context, lab, node, control, iface string, up bool) error {
	state := "down"
	if up {
		state = "up"
	}
	r, err := c.Exec(ctx, lab, node, control, 30*time.Second, nil, []string{"ip", "link", "set", iface, state})
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
