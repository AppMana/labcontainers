// Package containerd provides native runtime operations on an existing guest.
// It never selects, downloads, installs, or restarts a runtime implicitly.
package containerd

import (
	"context"
	"fmt"
	"strings"

	"github.com/appmana/labcontainers/pkg/rig"
)

// Images addresses an explicitly selected ctr CLI. Command may be a standalone
// ctr with global flags or a distribution wrapper such as {"k0s", "ctr"}.
// Snapshotter is required: host defaults must not decide Windows unpacking.
type Images struct {
	Command     []string
	Snapshotter string
	// LocalImport uses ctr's client-side importer instead of the transfer API.
	LocalImport bool
}

func (p Images) validate(values []string) error {
	if len(p.Command) == 0 || p.Command[0] == "" || p.Snapshotter == "" || len(values) == 0 {
		return fmt.Errorf("explicit runtime command, snapshotter, and inputs required")
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.HasPrefix(value, "-") || strings.ContainsRune(value, 0) {
			return fmt.Errorf("invalid image input %q", value)
		}
	}
	return nil
}

func (p Images) exec(ctx context.Context, node rig.Commands, args ...string) ([]byte, error) {
	argv := append(append([]string(nil), p.Command...), args...)
	out, err := node.Exec(ctx, argv...)
	if err != nil {
		return nil, fmt.Errorf("%s: runtime images: %w", node.Name(), err)
	}
	return out, nil
}

// Import loads caller-staged archives without a shell, downloads or globs.
// noUnpack allows importing HostProcess/support bundles without preparing
// ordinary container layers. False imports and unpacks the selected platform.
// Inputs must already have been verified by the caller's artifact boundary.
func (p Images) Import(ctx context.Context, node rig.Commands, archives []string, noUnpack bool) error {
	if err := p.validate(archives); err != nil {
		return err
	}
	for _, archive := range archives {
		args := []string{"images", "import", "--all-platforms"}
		if p.LocalImport {
			args = append(args, "--local")
		}
		if noUnpack {
			args = append(args, "--no-unpack")
		}
		args = append(args, "--snapshotter", p.Snapshotter, archive)
		if _, err := p.exec(ctx, node, args...); err != nil {
			return err
		}
	}
	return nil
}

// RequireReady performs one native completeness/unpack check. It neither
// repairs missing images nor starts workloads; the caller owns deadlines.
func (p Images) RequireReady(ctx context.Context, node rig.Commands, images []string) error {
	if err := p.validate(images); err != nil {
		return err
	}
	out, err := p.exec(ctx, node, "images", "check", "--snapshotter", p.Snapshotter, "--quiet")
	if err != nil {
		return err
	}
	ready := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		ready[strings.TrimSpace(line)] = true
	}
	for _, image := range images {
		if !ready[image] {
			return fmt.Errorf("%s: image %q is not complete and unpacked with snapshotter %s", node.Name(), image, p.Snapshotter)
		}
	}
	return nil
}
