package capi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/appmana/labcontainers/pkg/kubernetes/kube"
)

// Component is a released manifest a lab installs without registry access.
type Component struct {
	Name, URL, Namespace string
	// Deployments must roll out before the next component is installed.
	Deployments []string
}

// CertManagerVersion is the cert-manager release CertManager installs.
const CertManagerVersion = "v1.16.2"

// CertManager issues the webhook certificates Cluster API needs, so it is
// installed first.
func CertManager() Component {
	return Component{
		Name:        "cert-manager",
		URL:         "https://github.com/cert-manager/cert-manager/releases/download/" + CertManagerVersion + "/cert-manager.yaml",
		Namespace:   "cert-manager",
		Deployments: []string{"cert-manager", "cert-manager-webhook", "cert-manager-cainjector"},
	}
}

// ClusterAPI is the core Cluster API release at version.
func ClusterAPI(version string) Component {
	return Component{
		Name:        "cluster-api",
		URL:         "https://github.com/kubernetes-sigs/cluster-api/releases/download/" + version + "/cluster-api-components.yaml",
		Namespace:   "capi-system",
		Deployments: []string{"capi-controller-manager"},
	}
}

// ImageLoader carries one image onto every node of the lab.
type ImageLoader func(ctx context.Context, image string) error

// Install installs components in order. Each manifest is fetched once into
// cacheDir (the host has a route out; the lab may not), every image it names
// is carried in with load, the manifest is applied with clusterctl's default
// substitution, and its Deployments must roll out before the next component.
func Install(ctx context.Context, client *kube.Client, cacheDir string, load ImageLoader, components ...Component) error {
	for _, c := range components {
		manifest, err := fetch(ctx, cacheDir, c.Name+".yaml", c.URL)
		if err != nil {
			return err
		}
		images := kube.ImagesIn(manifest)
		if len(images) == 0 {
			return fmt.Errorf("%s's manifest names no images", c.Name)
		}
		for _, image := range images {
			if err := load(ctx, image); err != nil {
				return fmt.Errorf("carrying %s in: %w", image, err)
			}
		}
		if err := client.Apply(ctx, SubstituteDefaults(manifest)); err != nil {
			return fmt.Errorf("installing %s: %w", c.Name, err)
		}
		for _, d := range c.Deployments {
			if _, err := client.Run(ctx, "-n", c.Namespace, "rollout", "status", "deployment/"+d, "--timeout=6m"); err != nil {
				return fmt.Errorf("%s's %s did not become available: %w", c.Name, d, err)
			}
		}
	}
	return nil
}

func fetch(ctx context.Context, cacheDir, name, url string) ([]byte, error) {
	cached := filepath.Join(cacheDir, name)
	if body, err := os.ReadFile(cached); err == nil && len(body) > 0 {
		return body, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: %s", name, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(cached, body, 0o644); err != nil {
		return nil, err
	}
	return body, nil
}

// clusterctlVar matches the variables Cluster API's release files carry, in
// the form ${NAME} or ${NAME:=default}.
var clusterctlVar = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::=([^}]*))?\}`)

// SubstituteDefaults resolves the variables a published Cluster API
// components file leaves for clusterctl, as clusterctl does at install time:
// the environment first, then the stated default. Applied raw, the
// controller receives "${CAPI_INSECURE_DIAGNOSTICS:=false}" as a flag value
// and crash-loops. A variable with no default is left as it is, so something
// genuinely unset fails loudly instead of becoming an empty string.
func SubstituteDefaults(manifest []byte) []byte {
	return clusterctlVar.ReplaceAllFunc(manifest, func(m []byte) []byte {
		groups := clusterctlVar.FindSubmatch(m)
		if value, ok := os.LookupEnv(string(groups[1])); ok {
			return []byte(value)
		}
		if groups[2] != nil {
			return groups[2]
		}
		return m
	})
}
