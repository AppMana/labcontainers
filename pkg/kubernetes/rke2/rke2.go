// Package rke2 invokes RKE2's native installer on an existing node. The caller
// prepares and stages the installer and release files; this package selects no
// release, node role, runtime, network, or installation environment defaults.
package rke2

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/appmana/labcontainers/pkg/rig"
)

// Install invokes a staged installer using env's native NAME=value arguments.
// Values are passed unchanged, not interpolated into a shell command. The
// installer is caller-owned executable code and determines installation side
// effects. For offline tar installation, explicitly pass the native
// INSTALL_RKE2_ARTIFACT_PATH, INSTALL_RKE2_METHOD, INSTALL_RKE2_TYPE and
// INSTALL_RKE2_VERSION variables. This helper does not stage files, infer reuse,
// or issue a separate service start operation.
func Install(ctx context.Context, node rig.Commands, installer string, environment ...string) error {
	if !path.IsAbs(installer) || path.Clean(installer) == "/" {
		return fmt.Errorf("RKE2 installer must be an absolute file path")
	}
	for _, assignment := range environment {
		key, _, ok := strings.Cut(assignment, "=")
		if !ok || key == "" || strings.HasPrefix(key, "-") || strings.ContainsRune(assignment, '\x00') {
			return fmt.Errorf("installer environment must contain NAME=value assignments")
		}
	}
	argv := append([]string{"env", "--"}, environment...)
	argv = append(argv, installer)
	out, err := node.Exec(ctx, argv...)
	if err != nil {
		return fmt.Errorf("%s: RKE2 installer: %w: %s", node.Name(), err, out)
	}
	return nil
}

// Ready performs one authenticated API readiness attempt using explicit native
// paths. The caller owns retry timing. It does not prove pod connectivity.
func Ready(ctx context.Context, node rig.Commands, kubectl, kubeconfig string) error {
	for _, filename := range []string{kubectl, kubeconfig} {
		if !path.IsAbs(filename) || path.Clean(filename) == "/" {
			return fmt.Errorf("kubectl and kubeconfig require absolute file paths")
		}
	}
	if _, err := node.Exec(ctx, "test", "-f", kubeconfig); err != nil {
		return fmt.Errorf("%s: kubeconfig unavailable: %w", node.Name(), err)
	}
	if _, err := node.Exec(ctx, kubectl, "--kubeconfig", kubeconfig, "get", "--raw", "/readyz"); err != nil {
		return fmt.Errorf("%s: readyz: %w", node.Name(), err)
	}
	return nil
}
