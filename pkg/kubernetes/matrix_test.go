package kubernetes

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDeclarativeWindowsBGPRequiresPinsAndVerifiedBytes(t *testing.T) {
	data := []byte(`{"apiVersion":"v1","kind":"List","items":[]}`)
	newSelection := func() Selection {
		s := baseSelection()
		s.CNI, s.WindowsWorkers = CNICalicoBGP, true
		s.WindowsBGP = &WindowsBGPCapability{
			DeclarativeManifests: &ArtifactPin{Version: "bgp-1", SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), SourceRevision: strings.Repeat("b", 40)},
			RRASTooling:          ArtifactPin{Version: "rras-1", SHA256: digest},
			CalicoWindowsImage:   ArtifactPin{Version: "v3.32.2", SHA256: digest},
		}
		return s
	}
	for _, mutate := range []func(*Selection){
		func(s *Selection) { s.WindowsBGP.GeneratorSourceRevision = strings.Repeat("a", 40) },
		func(s *Selection) { s.WindowsBGP.DeclarativeManifests.SHA256 = "" },
		func(s *Selection) { s.WindowsBGP.DeclarativeManifests.SourceRevision = "" },
		func(s *Selection) { s.WindowsBGP.DeclarativeManifests.Version = "latest" },
		func(s *Selection) { s.WindowsBGP.RRASTooling.SHA256 = "" },
		func(s *Selection) { s.WindowsBGP.CalicoWindowsImage.SHA256 = "" },
		func(s *Selection) { s.WindowsWorkers = false },
		func(s *Selection) { s.CNI = CNICalicoVXLAN },
	} {
		s := newSelection()
		mutate(&s)
		if s.Validate() == nil {
			t.Fatal("accepted incomplete or ambiguous manifest capability")
		}
	}
	s := newSelection()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := s.WindowsBGP.VerifyDeclarativeManifests(data); err != nil {
		t.Fatal(err)
	}
	for _, wrong := range [][]byte{nil, []byte("{}"), append(append([]byte(nil), data...), '\n')} {
		if s.WindowsBGP.VerifyDeclarativeManifests(wrong) == nil {
			t.Fatal("accepted altered manifest bytes")
		}
	}
	s.WindowsBGP.GeneratorSourceRevision = strings.Repeat("a", 40)
	if s.WindowsBGP.VerifyDeclarativeManifests(data) == nil {
		t.Fatal("accepted two owners")
	}
	s.WindowsBGP.DeclarativeManifests = nil
	if s.WindowsBGP.VerifyDeclarativeManifests(data) == nil {
		t.Fatal("accepted missing manifest pin")
	}
}

func TestStockK0sWindowsBGPDeclarativeManifests(t *testing.T) {
	// Use the serialized public API to reproduce rejection before adding the
	// capability. The manifests belong to the caller, not the k0s executable.
	wire := `{"Distribution":"k0s","KubernetesVersion":"1.36.4",
	"DistributionBinary":{"Version":"v1.36.4+k0s.1","SHA256":"` + digest + `","SourceRevision":"` + strings.Repeat("a", 40) + `"},
	"CNI":"calico-bgp","WindowsWorkers":true,"WindowsBGP":{
	"DeclarativeManifests":{"Version":"calico-bgp-3.32.2","SHA256":"` + digest + `","SourceRevision":"` + strings.Repeat("b", 40) + `"},
	"RRASTooling":{"Version":"rras-1","SHA256":"` + digest + `"},
	"CalicoWindowsImage":{"Version":"v3.32.2-post.2","SHA256":"` + digest + `"}}}`
	var selection Selection
	if err := json.Unmarshal([]byte(wire), &selection); err != nil {
		t.Fatal(err)
	}
	if err := selection.Validate(); err != nil {
		t.Fatalf("stock distribution with independently pinned manifests: %v", err)
	}
}

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

func TestArtifactPinVersionLabels(t *testing.T) {
	for _, version := range []string{"latest", "stable", "main", "nightly", "1.36.*", "release"} {
		if (ArtifactPin{Version: version, SHA256: digest}).validate("test") == nil {
			t.Fatalf("accepted floating or non-version label %q", version)
		}
	}
	for _, version := range []string{"windows-x64-v3.32.0", "rras-tools-1", "v1.36.2+k0s.0"} {
		if err := (ArtifactPin{Version: version, SHA256: digest}).validate("test"); err != nil {
			t.Fatalf("rejected immutable label %q: %v", version, err)
		}
	}
}
