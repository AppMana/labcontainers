package k0s

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/appmana/labcontainers/pkg/rig"
	"github.com/appmana/labcontainers/pkg/wait"
	native "github.com/k0sproject/k0s/pkg/apis/k0s/v1beta1"
)

// Member roles.
const (
	Controller = "controller"
	Worker     = "worker"
)

const (
	binaryPath = "/usr/local/bin/k0s"
	configPath = "/etc/k0s/k0s.yaml"
	tokenPath  = "/etc/k0s/token"
)

// Member is one node of a site.
type Member struct {
	Node rig.Commands
	// Role is Controller or Worker.
	Role string
	// Config is a controller's native configuration, written unchanged.
	// Workers take theirs from the controllers and must leave it nil.
	Config *native.ClusterConfig
	// Args are passed to `k0s install <role>` unchanged, after the join
	// token and before the configuration path.
	Args []string
}

// Site builds a multi-node k0s cluster from a prepared, pinned binary. The
// first member must be a controller; it is started and serving before any
// other member joins, and each joining controller serves before the next
// member joins, because adding an etcd member changes quorum.
type Site struct {
	// Binary is the k0s executable every member runs. Callers supply a
	// content-pinned artifact; Site never downloads one.
	Binary []byte
	// Version is the exact `k0s version` every member must report.
	Version string
	Members []Member
	// ReadyWithin bounds each wait for a controller's API.
	ReadyWithin time.Duration
}

// Build carries the binary, checks every member's version before installing
// anything, then starts the first controller and joins the rest in order.
// Members already installed as a k0s service are started, not reinstalled.
func (s Site) Build(ctx context.Context) error {
	if len(s.Members) == 0 || s.Members[0].Role != Controller {
		return fmt.Errorf("a site's first member must be a controller")
	}
	for _, m := range s.Members {
		if (m.Role == Controller) != (m.Config != nil) {
			return fmt.Errorf("%s: controllers need a configuration and workers take none", m.Node.Name())
		}
		if err := m.Node.Put(ctx, bytes.NewReader(s.Binary), binaryPath, 0o755); err != nil {
			return fmt.Errorf("%s: carrying k0s: %w", m.Node.Name(), err)
		}
	}
	for _, m := range s.Members {
		if err := VerifyVersion(ctx, m.Node, s.Version); err != nil {
			return err
		}
	}
	first := s.Members[0]
	if err := s.install(ctx, first, false); err != nil {
		return err
	}
	if err := s.waitReady(ctx, first.Node); err != nil {
		return err
	}
	for _, m := range s.Members[1:] {
		if err := s.join(ctx, first.Node, m); err != nil {
			return err
		}
		if m.Role == Controller {
			if err := s.waitReady(ctx, m.Node); err != nil {
				return err
			}
		}
	}
	return nil
}

// VerifyVersion requires the node's k0s to report exactly version.
func VerifyVersion(ctx context.Context, node rig.Commands, version string) error {
	out, err := node.Exec(ctx, "k0s", "version")
	if err != nil {
		return fmt.Errorf("%s: reading k0s version: %w", node.Name(), err)
	}
	if got := strings.TrimSpace(string(out)); got != version {
		return fmt.Errorf("%s: k0s version %q does not match %s", node.Name(), got, version)
	}
	return nil
}

// InstalledDigest is the SHA-256 of the k0s binary every node carries. A
// site whose nodes carry different binaries is an error.
func InstalledDigest(ctx context.Context, nodes ...rig.Commands) (string, error) {
	var pin, first string
	for _, node := range nodes {
		out, err := node.Exec(ctx, "sha256sum", binaryPath)
		if err != nil {
			return "", fmt.Errorf("%s: hashing k0s: %w", node.Name(), err)
		}
		fields := strings.Fields(string(out))
		if len(fields) == 0 {
			return "", fmt.Errorf("%s reported no k0s digest", node.Name())
		}
		switch {
		case pin == "":
			pin, first = fields[0], node.Name()
		case fields[0] != pin:
			return "", fmt.Errorf("%s carries k0s %s, %s carries %s", node.Name(), fields[0], first, pin)
		}
	}
	return pin, nil
}

// join mints a role-specific token on the first controller, which must still
// be serving: a controller that has just joined can restart the one that
// admitted it.
func (s Site) join(ctx context.Context, first rig.Commands, m Member) error {
	if installed(ctx, m.Node) {
		return start(ctx, m.Node)
	}
	if err := s.waitReady(ctx, first); err != nil {
		return err
	}
	out, err := first.Exec(ctx, "k0s", "token", "create", "--role", m.Role)
	if err != nil {
		return fmt.Errorf("%s: minting a %s token: %w", first.Name(), m.Role, err)
	}
	token := strings.TrimSpace(strings.ReplaceAll(string(out), "\r", ""))
	if token == "" {
		return fmt.Errorf("%s printed no %s token", first.Name(), m.Role)
	}
	if _, err := m.Node.Exec(ctx, "mkdir", "-p", "/etc/k0s"); err != nil {
		return err
	}
	if err := m.Node.Put(ctx, strings.NewReader(token), tokenPath, 0o600); err != nil {
		return err
	}
	return s.install(ctx, m, true)
}

// install writes a controller's configuration, installs the k0s service
// unless it already exists, and starts it. A service rather than a process,
// so a rebooted node comes back through the distribution's own supervision.
func (s Site) install(ctx context.Context, m Member, joining bool) error {
	if !installed(ctx, m.Node) {
		args := []string{m.Role}
		if joining {
			args = append(args, "--token-file="+tokenPath)
		}
		args = append(args, m.Args...)
		if m.Config != nil {
			if err := WriteConfig(ctx, m.Node, configPath, m.Config); err != nil {
				return err
			}
			args = append(args, "--config="+configPath)
		}
		if err := Install(ctx, m.Node, args...); err != nil {
			return err
		}
	}
	return start(ctx, m.Node)
}

func installed(ctx context.Context, node rig.Commands) bool {
	for _, unit := range []string{"k0scontroller", "k0sworker"} {
		if _, err := node.Exec(ctx, "test", "-f", "/etc/systemd/system/"+unit+".service"); err == nil {
			return true
		}
	}
	return false
}

func start(ctx context.Context, node rig.Commands) error {
	if out, err := node.Exec(ctx, "k0s", "start"); err != nil {
		return fmt.Errorf("%s: k0s start: %w: %s", node.Name(), err, out)
	}
	return nil
}

func (s Site) waitReady(ctx context.Context, node rig.Commands) error {
	within := s.ReadyWithin
	if within == 0 {
		within = 5 * time.Minute
	}
	return wait.Until(ctx, within, node.Name()+"'s k0s never served a ready API", func(ctx context.Context) error {
		return Ready(ctx, node)
	})
}
