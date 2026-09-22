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
	fixture "github.com/appmana/labcontainers/pkg/kubernetes/k0s"
	native "github.com/k0sproject/k0s/pkg/apis/k0s/v1beta1"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/links"
	"github.com/srl-labs/containerlab/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This qualifies native offline import only. No CNI or kube-proxy is installed;
// it must not be used as evidence of a working Kubernetes pod/service network.
func TestLiveK0sAirgapImport(t *testing.T) {
	image := os.Getenv("LABCONTAINERS_K0S_AIRGAP_VM_IMAGE")
	if image == "" {
		t.Skip("set LABCONTAINERS_K0S_AIRGAP_VM_IMAGE and the explicit K0S_AIRGAP artifact inputs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	dir := t.TempDir()
	// Copy while hashing so the media contains the verified bytes, even if a
	// caller replaces the source path later. Large bundles are never buffered.
	for _, a := range []struct{ env, name string }{{"K0S_AIRGAP_BINARY", "k0s"}, {"K0S_AIRGAP_BUNDLE", "images.tar"}} {
		src, err := os.Open(os.Getenv(a.env))
		if err != nil {
			t.Fatal(err)
		}
		dst, err := os.Create(filepath.Join(dir, a.name))
		if err != nil {
			src.Close()
			t.Fatal(err)
		}
		h := sha256.New()
		_, copyErr := io.Copy(io.MultiWriter(dst, h), src)
		src.Close()
		closeErr := dst.Close()
		if copyErr != nil {
			t.Fatal(copyErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		want := os.Getenv(a.env + "_SHA256")
		if len(want) != 64 || fmt.Sprintf("%x", h.Sum(nil)) != want {
			t.Fatalf("%s checksum mismatch", a.env)
		}
	}
	expectedImage := os.Getenv("K0S_AIRGAP_EXPECTED_IMAGE")
	if expectedImage == "" {
		t.Fatal("K0S_AIRGAP_EXPECTED_IMAGE is required")
	}
	iso := filepath.Join(dir, "airgap.iso")
	if out, err := exec.CommandContext(ctx, "xorriso", "-as", "mkisofs", "-quiet", "-R", "-o", iso, filepath.Join(dir, "k0s"), filepath.Join(dir, "images.tar")).CombinedOutput(); err != nil {
		t.Fatalf("media: %v: %s", err, out)
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
		Defaults: &types.NodeDefinition{NetworkMode: "none", ImagePullPolicy: "Never"},
		Nodes: map[string]*types.NodeDefinition{
			"vm": {Kind: "generic_vm", Image: image, Binds: []string{iso + ":/airgap.iso:ro"}, Env: map[string]string{
				"QEMU_MEMORY": "4096", "QEMU_SMP": "4",
				"QEMU_ADDITIONAL_ARGS": "-drive file=/airgap.iso,format=raw,if=none,id=airgap,readonly=on -device virtio-blk-pci,drive=airgap,serial=lc-airgap",
			}},
			"peer": {Kind: "linux", Image: "alpine:3.20"},
		},
		Links: []*links.LinkDefinition{{Link: &links.LinkBriefRaw{Endpoints: []string{"vm:eth1", "peer:eth1"}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	lab, err := c.Start(ctx, &labv1.LabSpec{Topology: source, Nodes: map[string]*labv1.NodeExtension{"vm": {Control: "qga"}}}, 12*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("retained evidence: %s", lab.Artifacts())
	node := lab.Node("vm")
	defer func() {
		if t.Failed() {
			diagCtx, done := context.WithTimeout(context.Background(), 20*time.Second)
			defer done()
			out, err := node.Commands().Exec(diagCtx, "journalctl", "-u", "k0scontroller", "--no-pager", "-n", "80")
			t.Logf("k0s diagnostics: %s (%v)", out, err)
		}
	}()
	retry := func(argv ...string) []byte {
		t.Helper()
		for {
			out, err := node.Commands().Exec(ctx, argv...)
			if err == nil {
				return out
			}
			select {
			case <-ctx.Done():
				t.Fatalf("%v: %s: %v", argv, out, err)
				return nil
			case <-time.After(2 * time.Second):
			}
		}
	}
	retry("test", "-b", "/dev/disk/by-id/virtio-lc-airgap")
	if out, err := node.Commands().Exec(ctx, "sh", "-ec", `
iface=$(ls /sys/class/net | grep -v '^lo$')
test "$(printf '%s\n' "$iface" | wc -l)" = 1
ip link set "$iface" up
ip addr add 192.0.2.10/24 dev "$iface"
test -z "$(ip -4 route show default)"
test -z "$(ip -6 route show default)"
test ! -e /var/lib/k0s
mkdir -p /mnt/airgap /var/lib/k0s/images
mount -o ro /dev/disk/by-id/virtio-lc-airgap /mnt/airgap
install -m 0755 /mnt/airgap/k0s /usr/local/bin/k0s
cp /mnt/airgap/images.tar /var/lib/k0s/images/prepared.tar
`); err != nil {
		t.Fatalf("stage: %s: %v", out, err)
	}
	config := &native.ClusterConfig{TypeMeta: metav1.TypeMeta{APIVersion: native.ClusterConfigAPIVersion, Kind: native.ClusterConfigKind}, Spec: &native.ClusterSpec{
		API:     &native.APISpec{Address: "192.0.2.10", SANs: []string{"192.0.2.10"}},
		Network: &native.Network{Provider: "custom", PodCIDR: "10.244.0.0/16", ServiceCIDR: "10.96.0.0/12"},
	}}
	if err := fixture.WriteConfig(ctx, node.Commands(), "/etc/k0s/k0s.yaml", config); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Install(ctx, node.Commands(), "controller", "--single", "--config=/etc/k0s/k0s.yaml", "--disable-components=network-provider,kube-proxy,coredns,metrics-server,konnectivity-server,autopilot,windows-node", "--kubelet-extra-args=--node-ip=192.0.2.10"); err != nil {
		t.Fatal(err)
	}
	if _, err := node.Commands().Exec(ctx, "k0s", "start"); err != nil {
		t.Fatal(err)
	}
	// The bundle reconciler, not this test, must perform the import. Never run
	// ctr images import here: that would bypass the behavior being qualified.
	for {
		out, err := node.Commands().Exec(ctx, "k0s", "ctr", "--namespace=k8s.io", "images", "list", "-q")
		for _, line := range strings.Split(string(out), "\n") {
			if err == nil && line == expectedImage {
				if out, err := node.Commands().Exec(ctx, "sh", "-ec", `test -z "$(ip -4 route show default)"; test -z "$(ip -6 route show default)"; test "$(ls /sys/class/net | grep -vc '^lo$')" = 1`); err != nil {
					t.Fatalf("post-start isolation: %s: %v", out, err)
				}
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("image not imported: %s (%v)", out, err)
		case <-time.After(2 * time.Second):
		}
	}
}
