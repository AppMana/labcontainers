package k0sctl

import (
	"strings"
	"testing"

	native "github.com/k0sproject/k0s/pkg/apis/k0s/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func config(podCIDR string) *native.ClusterConfig {
	return &native.ClusterConfig{
		TypeMeta: metav1.TypeMeta{APIVersion: native.ClusterConfigAPIVersion, Kind: native.ClusterConfigKind},
		Spec:     &native.ClusterSpec{Network: &native.Network{Provider: "calico", PodCIDR: podCIDR}},
	}
}

func TestDocumentCarriesHostsVersionAndNativeConfigurationUnchanged(t *testing.T) {
	cluster := Cluster{Name: "lab", Version: "1.36.4+k0s.1", BinaryPath: "/stage/k0s", Config: config("10.244.0.0/16"), Hosts: []Host{
		{Address: "192.0.2.10", User: "operator", Role: ControllerWorker, NoTaints: true, InstallFlags: []string{"--disable-components=metrics-server"}},
		{Address: "192.0.2.11", User: "operator", Role: Worker, Port: 2222, KeyPath: "/keys/id"},
	}}
	data, err := cluster.YAML()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		APIVersion string `json:"apiVersion"`
		Spec       struct {
			Hosts []struct {
				SSH struct {
					Address, User, KeyPath string
					Port                   int
				} `json:"ssh"`
				Role          string
				InstallFlags  []string
				NoTaints      bool
				UploadBinary  bool
				K0sBinaryPath string
			}
			K0s struct {
				Version       string
				DynamicConfig bool
				Config        native.ClusterConfig
			}
		}
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	first, second := doc.Spec.Hosts[0], doc.Spec.Hosts[1]
	if doc.APIVersion != "k0sctl.k0sproject.io/v1beta1" || doc.Spec.K0s.Version != "1.36.4+k0s.1" || doc.Spec.K0s.DynamicConfig {
		t.Fatalf("header: %s", data)
	}
	if first.SSH.Address != "192.0.2.10" || first.SSH.Port != 22 || !first.NoTaints || !first.UploadBinary || first.K0sBinaryPath != "/stage/k0s" || first.InstallFlags[0] != "--disable-components=metrics-server" {
		t.Fatalf("first host: %+v", first)
	}
	if second.SSH.Port != 2222 || second.SSH.KeyPath != "/keys/id" || second.Role != Worker || second.NoTaints {
		t.Fatalf("second host: %+v", second)
	}
	if doc.Spec.K0s.Config.Spec.Network.PodCIDR != "10.244.0.0/16" || doc.Spec.K0s.Config.Kind != native.ClusterConfigKind {
		t.Fatalf("configuration changed: %s", data)
	}
	// An upgrade is the same document with another version and configuration.
	cluster.Version, cluster.Config = "1.37.2+k0s.1", config("10.245.0.0/16")
	upgraded, err := cluster.YAML()
	if err != nil || !strings.Contains(string(upgraded), "version: 1.37.2+k0s.1") || !strings.Contains(string(upgraded), "10.245.0.0/16") {
		t.Fatalf("%v\n%s", err, upgraded)
	}
}

func TestDocumentRejectsWhatK0sctlWouldMisread(t *testing.T) {
	good := Cluster{Name: "lab", Version: "1.36.4+k0s.1", Config: config("10.244.0.0/16"), Hosts: []Host{{Address: "192.0.2.10", User: "u", Role: Controller}}}
	if _, err := good.YAML(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Cluster){
		"v-prefixed version": func(c *Cluster) { c.Version = "v1.36.4+k0s.1" },
		"minor version":      func(c *Cluster) { c.Version = "1.36" },
		"no configuration":   func(c *Cluster) { c.Config = nil },
		"worker first":       func(c *Cluster) { c.Hosts[0].Role = Worker },
		"host name":          func(c *Cluster) { c.Hosts[0].Address = "controller.example" },
		"duplicate host":     func(c *Cluster) { c.Hosts = append(c.Hosts, c.Hosts[0]) },
		"unknown role":       func(c *Cluster) { c.Hosts = append(c.Hosts, Host{Address: "192.0.2.11", User: "u", Role: "etcd"}) },
		"untainted worker": func(c *Cluster) {
			c.Hosts = append(c.Hosts, Host{Address: "192.0.2.11", User: "u", Role: Worker, NoTaints: true})
		},
		"no user": func(c *Cluster) { c.Hosts[0].User = "" },
	} {
		c := good
		c.Hosts = append([]Host(nil), good.Hosts...)
		mutate(&c)
		if _, err := c.YAML(); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
}
