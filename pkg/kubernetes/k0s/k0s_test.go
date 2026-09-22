package k0s

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/appmana/labcontainers/pkg/rig"
	native "github.com/k0sproject/k0s/pkg/apis/k0s/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type node struct {
	rig.Node
	commands [][]string
	filename string
	mode     fs.FileMode
	data     []byte
	err      error
	failAt   int
}

func (n *node) Name() string { return "explicit-node" }
func (n *node) Exec(_ context.Context, args ...string) ([]byte, error) {
	n.commands = append(n.commands, append([]string(nil), args...))
	if n.failAt == 0 || n.failAt == len(n.commands) {
		return []byte("native diagnostic"), n.err
	}
	return nil, nil
}
func (n *node) Put(_ context.Context, src io.Reader, filename string, mode fs.FileMode) error {
	n.filename, n.mode = filename, mode
	var err error
	n.data, err = io.ReadAll(src)
	return err
}

func TestWriteNativeConfigWithoutAddingDefaults(t *testing.T) {
	n := &node{}
	cfg := &native.ClusterConfig{
		TypeMeta: metav1.TypeMeta{APIVersion: native.ClusterConfigAPIVersion, Kind: native.ClusterConfigKind},
		Spec: &native.ClusterSpec{
			API:     &native.APISpec{Address: "192.0.2.5", SANs: []string{"name: not YAML", "192.0.2.5"}},
			Network: &native.Network{Provider: "custom", PodCIDR: "10.20.0.0/16"},
		},
	}
	before := cfg.DeepCopy()
	if err := WriteConfig(context.Background(), n, "/etc/test/k0s.yaml", cfg); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, before) {
		t.Fatal("mutated caller's native config")
	}
	if n.filename != "/etc/test/k0s.yaml" || n.mode != 0o600 {
		t.Fatalf("wrong output: %s %o", n.filename, n.mode)
	}
	if !reflect.DeepEqual(n.commands, [][]string{{"mkdir", "-p", "/etc/test"}}) {
		t.Fatalf("unexpected node operations: %v", n.commands)
	}
	// Compare wire data rather than unmarshalling into native structs: their
	// UnmarshalJSON methods themselves apply upstream defaults.
	want, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := yaml.YAMLToJSON(n.data)
	if err != nil {
		t.Fatal(err)
	}
	var wantObj, gotObj any
	if err := json.Unmarshal(want, &wantObj); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &gotObj); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotObj, wantObj) {
		t.Fatalf("changed native object: %s", n.data)
	}
	for _, absent := range []string{"calico:", "nodeLocalLoadBalancing:", "images:", "telemetry:"} {
		if strings.Contains(string(n.data), absent) {
			t.Fatalf("inserted %s", absent)
		}
	}
}

func TestInvalidConfigurationDoesNotTouchNode(t *testing.T) {
	for _, path := range []string{"", "relative.yaml", "/"} {
		n := &node{}
		if err := WriteConfig(context.Background(), n, path, &native.ClusterConfig{}); err == nil {
			t.Fatal("accepted invalid path", path)
		}
		if len(n.commands) != 0 || n.filename != "" {
			t.Fatal("mutated node on validation failure")
		}
	}
	n := &node{}
	if err := WriteConfig(context.Background(), n, "/etc/k0s.yaml", nil); err == nil || len(n.commands) != 0 {
		t.Fatal("accepted nil config")
	}
}

func TestInstallPassesNativeArgumentsWithoutShellOrStart(t *testing.T) {
	n := &node{}
	args := []string{"controller", "--enable-worker", "--kubelet-extra-args=--node-ip=192.0.2.5 --node-labels=x=y", "--config=/path with spaces/config.yaml"}
	if err := Install(context.Background(), n, args...); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(n.commands, [][]string{append([]string{"k0s", "install"}, args...)}) {
		t.Fatalf("changed native arguments: %v", n.commands)
	}
	boom := errors.New("installation failed")
	n = &node{err: boom}
	if err := Install(context.Background(), n, "worker"); !errors.Is(err, boom) || !strings.Contains(err.Error(), "native diagnostic") {
		t.Fatalf("lost native failure: %v", err)
	}
	n = &node{}
	if err := Install(context.Background(), n); err == nil || len(n.commands) != 0 {
		t.Fatal("accepted absent install arguments")
	}
}

func TestReadyRequiresEachNativeProbe(t *testing.T) {
	for fail := 0; fail <= 3; fail++ {
		n := &node{failAt: fail}
		if fail > 0 {
			n.err = errors.New("not ready")
		}
		err := Ready(context.Background(), n)
		if fail == 0 {
			if err != nil || len(n.commands) != 3 {
				t.Fatalf("ready failed: %v, %v", err, n.commands)
			}
		} else if !errors.Is(err, n.err) || len(n.commands) != fail {
			t.Fatalf("did not stop at failed probe: %v, %v", err, n.commands)
		}
	}
}
