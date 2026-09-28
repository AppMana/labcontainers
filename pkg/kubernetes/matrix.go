// Package kubernetes defines explicit Kubernetes distribution/CNI selections.
// It does not create a topology, choose an artifact, or perform VM operations.
package kubernetes

import (
	"fmt"
	"regexp"
	"strings"
)

type Distribution string

const (
	DistributionK0s  Distribution = "k0s"
	DistributionRKE2 Distribution = "rke2"
)

type CNI string

const (
	CNICalicoVXLAN CNI = "calico-vxlan"
	CNICalicoBGP   CNI = "calico-bgp"
	CNIKubeRouter  CNI = "kuberouter"
)

// ArtifactPin identifies caller-prepared bytes. Version is a non-floating
// provenance label; SHA256 supplies immutable content identity and is verified
// before any VM operation.
type ArtifactPin struct {
	Version, SHA256 string
	// SourceRevision is the full lowercase Git commit of caller-built inputs.
	// It is required for explicitly labeled AppMana k0s forks; a binary hash
	// remains mandatory and is not replaced by source provenance.
	SourceRevision string
}

// WindowsBGPCapability identifies fork-specific inputs required to render and
// run Calico's unencapsulated Windows L2Bridge/BGP path. Stock k0s emits its
// Windows DaemonSet only for VXLAN. This remains a qualification candidate,
// not evidence that a VM gate passed.
type WindowsBGPCapability struct {
	GeneratorSourceRevision string
	RRASTooling             ArtifactPin
	CalicoWindowsImage      ArtifactPin
}
type Selection struct {
	Distribution       Distribution
	KubernetesVersion  string
	DistributionBinary ArtifactPin
	CNI                CNI
	WindowsWorkers     bool
	WindowsBGP         *WindowsBGPCapability
}

var (
	exactKubernetesVersion = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)
	sha256Hex              = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	k0sArtifactVersion     = regexp.MustCompile(`^v?([0-9]+\.[0-9]+\.[0-9]+)\+k0s\.[0-9]+(?:\.appmana\.([0-9a-f]{7,40}))?$`)
	sourceRevision         = regexp.MustCompile(`^[0-9a-f]{40}$`)
	versionDigit           = regexp.MustCompile(`[0-9]`)
)

func (p ArtifactPin) validate(name string) error {
	version := strings.TrimSpace(p.Version)
	floating := map[string]bool{"latest": true, "stable": true, "current": true, "main": true, "master": true, "nightly": true}
	if version == "" || strings.Contains(version, "*") || floating[strings.ToLower(version)] || !versionDigit.MatchString(version) || !sha256Hex.MatchString(p.SHA256) {
		return fmt.Errorf("%s requires a non-floating version label containing a digit and a 64-digit SHA256", name)
	}
	if p.SourceRevision != "" && !sourceRevision.MatchString(p.SourceRevision) {
		return fmt.Errorf("%s source revision must be a full 40-digit lowercase Git commit", name)
	}
	return nil
}

// Validate rejects inconsistent tuples before any prepared artifact is
// verified, staged, or executed. Acceptance describes configuration capability
// only; it does not claim that this exact release was qualified in a VM.
func (s Selection) Validate() error {
	if !exactKubernetesVersion.MatchString(s.KubernetesVersion) {
		return fmt.Errorf("an exact Kubernetes version (for example 1.34.7), not a minor series, is required")
	}
	if err := s.DistributionBinary.validate("distribution binary"); err != nil {
		return err
	}
	switch s.Distribution {
	case DistributionK0s:
		match := k0sArtifactVersion.FindStringSubmatch(s.DistributionBinary.Version)
		if len(match) != 3 || strings.TrimPrefix(s.KubernetesVersion, "v") != match[1] {
			return fmt.Errorf("k0s artifact version %q does not identify Kubernetes %q", s.DistributionBinary.Version, s.KubernetesVersion)
		}
		if match[2] != "" && (!sourceRevision.MatchString(s.DistributionBinary.SourceRevision) ||
			!strings.HasPrefix(s.DistributionBinary.SourceRevision, match[2])) {
			return fmt.Errorf("AppMana k0s fork version suffix must match its full artifact source revision")
		}
	case DistributionRKE2:
		return fmt.Errorf("RKE2 specialization matrix is not implemented; use the explicit native rke2 package")
	default:
		return fmt.Errorf("unsupported Kubernetes distribution %q", s.Distribution)
	}
	switch s.CNI {
	case CNICalicoVXLAN:
		if s.WindowsBGP != nil {
			return fmt.Errorf("Windows BGP capability is only valid with calico-bgp")
		}
	case CNIKubeRouter:
		if s.WindowsWorkers {
			return fmt.Errorf("k0s kube-router does not support Windows workers")
		}
		if s.WindowsBGP != nil {
			return fmt.Errorf("Windows BGP capability is only valid with calico-bgp")
		}
	case CNICalicoBGP:
		if !s.WindowsWorkers {
			if s.WindowsBGP != nil {
				return fmt.Errorf("Windows BGP capability supplied without Windows workers")
			}
			break
		}
		if s.WindowsBGP == nil || !sourceRevision.MatchString(s.WindowsBGP.GeneratorSourceRevision) {
			return fmt.Errorf("k0s Windows Calico BGP requires a 40-digit source revision for a generator that emits the Windows BGP DaemonSet")
		}
		if s.DistributionBinary.SourceRevision != "" && s.WindowsBGP.GeneratorSourceRevision != s.DistributionBinary.SourceRevision {
			return fmt.Errorf("Windows BGP generator source revision must match the distribution artifact source revision")
		}
		if err := s.WindowsBGP.RRASTooling.validate("Windows RRAS tooling"); err != nil {
			return err
		}
		if err := s.WindowsBGP.CalicoWindowsImage.validate("Calico Windows image"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported CNI %q", s.CNI)
	}
	return nil
}
