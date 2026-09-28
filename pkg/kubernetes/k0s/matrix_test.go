package k0s

import (
	"context"
	"reflect"
	"strings"
	"testing"

	matrix "github.com/appmana/labcontainers/pkg/kubernetes"
	native "github.com/k0sproject/k0s/pkg/apis/k0s/v1beta1"
)

const matrixDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func matrixSelection(cni matrix.CNI, windows bool) matrix.Selection {
	return matrix.Selection{Distribution: matrix.DistributionK0s, KubernetesVersion: "1.36.2", DistributionBinary: matrix.ArtifactPin{Version: "v1.36.2+k0s.0", SHA256: matrixDigest}, CNI: cni, WindowsWorkers: windows}
}

func TestConfigureNetworkUsesNativeK0sFields(t *testing.T) {
	tests := []struct {
		name     string
		cni      matrix.CNI
		provider string
		mode     native.CalicoMode
		overlay  string
	}{
		{"vxlan", matrix.CNICalicoVXLAN, "calico", native.CalicoModeVXLAN, "Always"},
		{"bgp", matrix.CNICalicoBGP, "calico", native.CalicoModeBIRD, "Never"},
		{"kube-router", matrix.CNIKubeRouter, "kuberouter", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &native.ClusterConfig{Spec: &native.ClusterSpec{Network: &native.Network{}}}
			if err := ConfigureNetwork(cfg, matrixSelection(tc.cni, false)); err != nil {
				t.Fatal(err)
			}
			if cfg.Spec.Network.Provider != tc.provider {
				t.Fatalf("provider=%q", cfg.Spec.Network.Provider)
			}
			if tc.cni == matrix.CNIKubeRouter {
				if cfg.Spec.Network.KubeRouter != nil || cfg.Spec.Network.Calico != nil {
					t.Fatal("wrong native kube-router fields")
				}
			} else if cfg.Spec.Network.Calico == nil || cfg.Spec.Network.Calico.Mode != tc.mode || cfg.Spec.Network.Calico.Overlay != tc.overlay || cfg.Spec.Network.KubeRouter != nil {
				t.Fatalf("wrong native Calico fields: %#v", cfg.Spec.Network.Calico)
			}
		})
	}
}

func TestConfigureNetworkRejectsBeforeMutation(t *testing.T) {
	cfg := &native.ClusterConfig{Spec: &native.ClusterSpec{Network: &native.Network{Provider: "custom"}}}
	before := cfg.DeepCopy()
	if err := ConfigureNetwork(cfg, matrixSelection(matrix.CNIKubeRouter, true)); err == nil {
		t.Fatal("accepted Windows kube-router")
	}
	if !reflect.DeepEqual(cfg, before) {
		t.Fatal("mutated config on rejected tuple")
	}
}

func TestConfigureNetworkAttestedWindowsBGPFork(t *testing.T) {
	const revision = "2a2a0880d35d8dfc5eb7eab58509da4611280648"
	s := matrixSelection(matrix.CNICalicoBGP, true)
	s.DistributionBinary = matrix.ArtifactPin{Version: "v1.36.2+k0s.0.appmana.2a2a088", SHA256: matrixDigest, SourceRevision: revision}
	s.WindowsBGP = &matrix.WindowsBGPCapability{GeneratorSourceRevision: revision,
		RRASTooling:        matrix.ArtifactPin{Version: "rras-1", SHA256: matrixDigest},
		CalicoWindowsImage: matrix.ArtifactPin{Version: "v3.32.0-fork.1", SHA256: matrixDigest}}
	cfg := &native.ClusterConfig{Spec: &native.ClusterSpec{Network: &native.Network{Provider: "custom"}}}
	if err := ConfigureNetwork(cfg, s); err != nil {
		t.Fatal(err)
	}
	if cfg.Spec.Network.Provider != "calico" || cfg.Spec.Network.Calico.Mode != native.CalicoModeBIRD || cfg.Spec.Network.Calico.Overlay != "Never" {
		t.Fatalf("fork selection lost native BGP fields: %+v", cfg.Spec.Network)
	}
	before := cfg.DeepCopy()
	s.WindowsBGP.GeneratorSourceRevision = strings.Repeat("a", 40)
	if err := ConfigureNetwork(cfg, s); err == nil {
		t.Fatal("accepted unrelated generator provenance")
	}
	if !reflect.DeepEqual(cfg, before) {
		t.Fatal("rejected fork mutated config")
	}
}

func TestWriteConfigWithoutSpecializationPreservesCustomProvider(t *testing.T) {
	cfg := &native.ClusterConfig{Spec: &native.ClusterSpec{Network: &native.Network{Provider: "custom"}}}
	n := &node{}
	if err := WriteConfig(context.Background(), n, "/etc/k0s/generic.yaml", cfg); err != nil {
		t.Fatal(err)
	}
	wire := string(n.data)
	if !strings.Contains(wire, "provider: custom") || strings.Contains(wire, "provider: calico") || strings.Contains(wire, "provider: kuberouter") {
		t.Fatalf("serialization implicitly specialized generic config: %s", wire)
	}
}
