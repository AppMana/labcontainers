package k0s

import (
	"reflect"
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

func TestGenericNativeConfigStillRequiresNoSpecialization(t *testing.T) {
	cfg := &native.ClusterConfig{Spec: &native.ClusterSpec{Network: &native.Network{Provider: "custom"}}}
	before := cfg.DeepCopy()
	if !reflect.DeepEqual(cfg, before) {
		t.Fatal("generic native config changed without ConfigureNetwork")
	}
}
