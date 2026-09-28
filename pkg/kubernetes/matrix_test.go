package kubernetes

import (
	"strings"
	"testing"
)

const digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func baseSelection() Selection {
	return Selection{Distribution: DistributionK0s, KubernetesVersion: "1.36.2", DistributionBinary: ArtifactPin{Version: "v1.36.2+k0s.0", SHA256: digest}, CNI: CNICalicoVXLAN}
}
func TestSelectionMatrix(t *testing.T) {
	bgp := &WindowsBGPCapability{GeneratorSourceRevision: strings.Repeat("a", 40), RRASTooling: ArtifactPin{Version: "rras-1", SHA256: digest}, CalicoWindowsImage: ArtifactPin{Version: "v3.32.0-fork.1", SHA256: digest}}
	tests := []struct {
		name string
		edit func(*Selection)
		want string
	}{
		{"linux vxlan", func(*Selection) {}, ""},
		{"windows vxlan", func(s *Selection) { s.WindowsWorkers = true }, ""},
		{"linux bgp", func(s *Selection) { s.CNI = CNICalicoBGP }, ""},
		{"windows bgp candidate", func(s *Selection) { s.CNI = CNICalicoBGP; s.WindowsWorkers = true; s.WindowsBGP = bgp }, ""},
		{"windows bgp stock", func(s *Selection) { s.CNI = CNICalicoBGP; s.WindowsWorkers = true }, "source revision"},
		{"linux kube-router", func(s *Selection) { s.CNI = CNIKubeRouter }, ""},
		{"windows kube-router", func(s *Selection) { s.CNI = CNIKubeRouter; s.WindowsWorkers = true }, "does not support Windows"},
		{"minor selector", func(s *Selection) { s.KubernetesVersion = "1.36.x" }, "exact Kubernetes version"},
		{"artifact mismatch", func(s *Selection) { s.DistributionBinary.Version = "v1.35.9+k0s.0" }, "does not identify"},
		{"rke2", func(s *Selection) { s.Distribution = DistributionRKE2; s.DistributionBinary.Version = "v1.36.2+rke2r1" }, "not implemented"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := baseSelection()
			tc.edit(&s)
			err := s.Validate()
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}
func TestWindowsBGPRequiresEveryPinnedInput(t *testing.T) {
	s := baseSelection()
	s.CNI = CNICalicoBGP
	s.WindowsWorkers = true
	s.WindowsBGP = &WindowsBGPCapability{GeneratorSourceRevision: strings.Repeat("a", 40), RRASTooling: ArtifactPin{Version: "rras-1", SHA256: digest}, CalicoWindowsImage: ArtifactPin{Version: "v3.32.0", SHA256: digest}}
	for _, mutate := range []func(*Selection){
		func(s *Selection) { s.WindowsBGP.GeneratorSourceRevision = "main" },
		func(s *Selection) { s.WindowsBGP.RRASTooling.SHA256 = "" },
		func(s *Selection) {
			s.WindowsBGP.CalicoWindowsImage.Version = "latest"
			s.WindowsBGP.CalicoWindowsImage.SHA256 = ""
		},
	} {
		copy := s
		capCopy := *s.WindowsBGP
		copy.WindowsBGP = &capCopy
		mutate(&copy)
		if copy.Validate() == nil {
			t.Fatal("accepted incomplete Windows BGP capability")
		}
	}
}
