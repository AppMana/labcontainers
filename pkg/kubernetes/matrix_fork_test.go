package kubernetes

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const generatorRevision = "2a2a0880d35d8dfc5eb7eab58509da4611280648"

func TestAttestedK0sForkVersion(t *testing.T) {
	for _, tc := range []struct {
		name, version, revision, generator string
		valid                              bool
	}{
		{"candidate", "v1.36.2+k0s.0.appmana.2a2a088", generatorRevision, generatorRevision, true},
		{"full commit", "v1.36.2+k0s.0.appmana." + generatorRevision, generatorRevision, generatorRevision, true},
		{"missing source", "v1.36.2+k0s.0.appmana.2a2a088", "", generatorRevision, false},
		{"wrong source", "v1.36.2+k0s.0.appmana.2a2a088", strings.Repeat("a", 40), generatorRevision, false},
		{"wrong generator", "v1.36.2+k0s.0.appmana.2a2a088", generatorRevision, strings.Repeat("a", 40), false},
		{"floating source", "v1.36.2+k0s.0.appmana.2a2a088", "main", generatorRevision, false},
		{"short source", "v1.36.2+k0s.0.appmana.2a2a088", "2a2a088", generatorRevision, false},
		{"floating suffix", "v1.36.2+k0s.0.appmana.main", generatorRevision, generatorRevision, false},
		{"short suffix", "v1.36.2+k0s.0.appmana.2a2a", generatorRevision, generatorRevision, false},
		{"arbitrary suffix", "v1.36.2+k0s.0.other.2a2a088", generatorRevision, generatorRevision, false},
		{"extra suffix", "v1.36.2+k0s.0.appmana.2a2a088.dirty", generatorRevision, generatorRevision, false},
		{"wrong Kubernetes", "v1.36.3+k0s.0.appmana.2a2a088", generatorRevision, generatorRevision, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := baseSelection()
			s.CNI = CNICalicoBGP
			s.WindowsWorkers = true
			// Use the public serialized pin so this behavioral regression can run
			// before adding the SourceRevision field to the existing API.
			wire := fmt.Sprintf(`{"Version":%q,"SHA256":%q,"SourceRevision":%q}`, tc.version, digest, tc.revision)
			if err := json.Unmarshal([]byte(wire), &s.DistributionBinary); err != nil {
				t.Fatal(err)
			}
			s.WindowsBGP = &WindowsBGPCapability{GeneratorSourceRevision: tc.generator, RRASTooling: ArtifactPin{Version: "rras-1", SHA256: digest}, CalicoWindowsImage: ArtifactPin{Version: "v3.32.0-fork.1", SHA256: digest}}
			if err := s.Validate(); (err == nil) != tc.valid {
				t.Fatalf("Validate()=%v, valid=%v", err, tc.valid)
			}
			if tc.valid {
				s.DistributionBinary.SHA256 = ""
				if s.Validate() == nil {
					t.Fatal("accepted fork without exact binary hash")
				}
			}
		})
	}
}
