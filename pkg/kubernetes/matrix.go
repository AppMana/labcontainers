// Package kubernetes defines explicit Kubernetes distribution/CNI selections.
// It does not create a topology, choose an artifact, or perform VM operations.
package kubernetes

import (
	"crypto/sha256"
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

// WindowsBGPCapability identifies explicit inputs for Windows L2Bridge/BGP.
// Stock k0s emits its Windows DaemonSet only for VXLAN; callers must select
// either a patched distribution generator or independently owned manifests.
// This describes capability, not evidence that a VM gate passed.
type WindowsBGPCapability struct {
	GeneratorSourceRevision string
	// DeclarativeManifests pins the serialized native Kubernetes objects that
	// the caller owns (for example through GitOps). It is mutually exclusive
	// with GeneratorSourceRevision. The caller must verify these bytes before
	// applying them and establish readiness; ConfigureNetwork does neither.
	DeclarativeManifests *ArtifactPin
	RRASTooling          ArtifactPin
	CalicoWindowsImage   ArtifactPin
}

// VerifyDeclarativeManifests verifies prepared manifest bytes without applying
// resources, opening a network, or treating a matching hash as readiness.
func (c WindowsBGPCapability) VerifyDeclarativeManifests(data []byte) error {
	if c.DeclarativeManifests == nil || c.GeneratorSourceRevision != "" {
		return fmt.Errorf("select only declarative Windows BGP manifests")
	}
	if err := c.DeclarativeManifests.validate("Windows BGP declarative manifests"); err != nil {
		return err
	}
	if !sourceRevision.MatchString(c.DeclarativeManifests.SourceRevision) {
		return fmt.Errorf("Windows BGP declarative manifests require a full source revision")
	}
	if len(data) == 0 || !strings.EqualFold(fmt.Sprintf("%x", sha256.Sum256(data)), c.DeclarativeManifests.SHA256) {
		return fmt.Errorf("Windows BGP declarative manifest checksum mismatch")
	}
	return nil
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
		if s.WindowsBGP == nil {
			return fmt.Errorf("k0s Windows Calico BGP requires a 40-digit source revision for a generator that emits the Windows BGP DaemonSet")
		}
		if manifests := s.WindowsBGP.DeclarativeManifests; manifests != nil {
			if s.WindowsBGP.GeneratorSourceRevision != "" {
				return fmt.Errorf("select either a Windows BGP distribution generator or declarative manifests, not both")
			}
			if err := manifests.validate("Windows BGP declarative manifests"); err != nil {
				return err
			}
			if !sourceRevision.MatchString(manifests.SourceRevision) {
				return fmt.Errorf("Windows BGP declarative manifests require a full source revision")
			}
		} else {
			if !sourceRevision.MatchString(s.WindowsBGP.GeneratorSourceRevision) {
				return fmt.Errorf("Windows BGP generator requires a full source revision")
			}
			if s.DistributionBinary.SourceRevision != "" && s.WindowsBGP.GeneratorSourceRevision != s.DistributionBinary.SourceRevision {
				return fmt.Errorf("Windows BGP generator source revision must match the distribution artifact source revision")
			}
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
