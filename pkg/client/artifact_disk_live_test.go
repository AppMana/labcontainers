package client

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/types"
)

// Artifact disks use native Containerlab binds and vrnetlab's existing QEMU
// arguments. No SDK disk schema, transfer service, or guest network is needed.
func TestLiveReadOnlyArtifactDisk(t *testing.T) {
	image := os.Getenv("LABCONTAINERS_ARTIFACT_DISK_IMAGE")
	if image == "" {
		t.Skip("set LABCONTAINERS_ARTIFACT_DISK_IMAGE to a prepared Linux QGA image")
	}
	xorriso, err := exec.LookPath("xorriso")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	dir := t.TempDir()
	payload := filepath.Join(dir, "payload.bin")
	f, err := os.Create(payload)
	if err != nil {
		t.Fatal(err)
	}
	// Sparse source, with distinct bytes at each end, exceeds the upload limit.
	size := int64(labv1.MaxPutBytes + (1 << 20))
	if err := f.Truncate(size); err != nil {
		f.Close()
		t.Fatal(err)
	}
	marker := []byte("artifact-boundary")
	for _, offset := range []int64{0, size - int64(len(marker))} {
		if _, err := f.WriteAt(marker, offset); err != nil {
			f.Close()
			t.Fatal(err)
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		t.Fatal(err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%x", hash.Sum(nil))
	iso := filepath.Join(dir, "artifacts.iso")
	if out, err := exec.CommandContext(ctx, xorriso, "-as", "mkisofs", "-quiet", "-R", "-V", "LC_ARTIFACTS", "-o", iso, payload).CombinedOutput(); err != nil {
		t.Fatalf("create artifact media: %v: %s", err, out)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	c, err := Launch(ctx, Options{LabdPath: filepath.Join(root, "bin", "labd")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()
	source, err := clab.Source(&core.Config{Topology: &types.Topology{
		Nodes: map[string]*types.NodeDefinition{"vm": {
			Kind: "generic_vm", Image: image, ImagePullPolicy: "Never", NetworkMode: "none",
			Binds: []string{iso + ":/artifacts.iso:ro"},
			Env:   map[string]string{"QEMU_ADDITIONAL_ARGS": "-drive file=/artifacts.iso,format=raw,if=none,id=artifacts,readonly=on -device virtio-blk-pci,drive=artifacts,serial=lc-artifacts"},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	lab, err := c.Start(ctx, &labv1.LabSpec{Topology: source, Nodes: map[string]*labv1.NodeExtension{"vm": {Control: "qga"}}}, 8*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("retained evidence: %s", lab.Artifacts())
	node := lab.Node("vm")
	for {
		result, err := node.Exec(ctx, "test", "-b", "/dev/disk/by-id/virtio-lc-artifacts")
		if err == nil && result.GetExitCode() == 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("artifact disk not ready: %v (%v)", ctx.Err(), err)
		case <-time.After(2 * time.Second):
		}
	}
	// Hash both the mounted file and a guest-local copy, not just the ISO on the
	// host. This is the same offline path usable for prepared image archives.
	result, err := node.Exec(ctx, "sh", "-ec", `
test "$(ls /sys/class/net)" = lo
test -z "$(ip -4 route show default)"
test -z "$(ip -6 route show default)"
test "$(blockdev --getro /dev/disk/by-id/virtio-lc-artifacts)" = 1
mkdir -p /mnt/lc-artifacts
mount -o ro /dev/disk/by-id/virtio-lc-artifacts /mnt/lc-artifacts
if touch /mnt/lc-artifacts/forbidden 2>/dev/null; then exit 1; fi
cp /mnt/lc-artifacts/payload.bin /var/tmp/lc-artifact-copy.bin
sha256sum /mnt/lc-artifacts/payload.bin /var/tmp/lc-artifact-copy.bin
`)
	if err != nil || result.GetExitCode() != 0 {
		t.Fatalf("guest verification: %v: %v", result, err)
	}
	lines := strings.Split(strings.TrimSpace(string(result.GetStdout())), "\n")
	if len(lines) != 2 {
		t.Fatalf("unexpected hashes: %q", result.GetStdout())
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, want+"  ") {
			t.Fatalf("artifact mismatch: %s, want %s", line, want)
		}
	}
}
