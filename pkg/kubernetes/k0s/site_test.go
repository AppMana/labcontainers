package k0s

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	native "github.com/k0sproject/k0s/pkg/apis/k0s/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// siteNode answers the k0s CLI the way an installed node does and records
// every command and file, in order, across all nodes of one site.
type siteNode struct {
	name      string
	version   string
	installed bool
	log       *[]string
	files     map[string]string
}

func (n *siteNode) Name() string { return n.name }

func (n *siteNode) Exec(_ context.Context, args ...string) ([]byte, error) {
	line := n.name + ": " + strings.Join(args, " ")
	*n.log = append(*n.log, line)
	switch {
	case len(args) >= 2 && args[0] == "k0s" && args[1] == "version":
		return []byte(n.version + "\n"), nil
	case len(args) == 3 && args[0] == "test" && args[1] == "-f":
		if n.installed && strings.HasPrefix(args[2], "/etc/systemd/system/k0s") {
			return nil, nil
		}
		return nil, errors.New("absent")
	case len(args) >= 3 && args[0] == "k0s" && args[1] == "token":
		return []byte("token-" + args[len(args)-1] + "\r\n"), nil
	}
	return nil, nil
}

func (n *siteNode) Pipe(ctx context.Context, _ io.Reader, args ...string) ([]byte, error) {
	return n.Exec(ctx, args...)
}

func (n *siteNode) Put(_ context.Context, src io.Reader, filename string, _ fs.FileMode) error {
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	n.files[filename] = string(data)
	*n.log = append(*n.log, n.name+": put "+filename)
	return nil
}

func controllerConfig(address string) *native.ClusterConfig {
	return &native.ClusterConfig{
		TypeMeta: metav1.TypeMeta{APIVersion: native.ClusterConfigAPIVersion, Kind: native.ClusterConfigKind},
		Spec:     &native.ClusterSpec{API: &native.APISpec{Address: address}},
	}
}

func newSite(version string, installed ...string) (Site, map[string]*siteNode, *[]string) {
	log := &[]string{}
	nodes := map[string]*siteNode{}
	for _, name := range []string{"cp1", "cp2", "w1"} {
		nodes[name] = &siteNode{name: name, version: version, log: log, files: map[string]string{}}
	}
	for _, name := range installed {
		nodes[name].installed = true
	}
	site := Site{
		Binary:  []byte("k0s-binary"),
		Version: "v1.36.4+k0s.1",
		Members: []Member{
			{Node: nodes["cp1"], Role: Controller, Config: controllerConfig("192.0.2.10"), Args: []string{"--enable-worker"}},
			{Node: nodes["cp2"], Role: Controller, Config: controllerConfig("192.0.2.11"), Args: []string{"--enable-worker"}},
			{Node: nodes["w1"], Role: Worker, Args: []string{"--kubelet-extra-args=--node-ip=192.0.2.20"}},
		},
		ReadyWithin: time.Second,
	}
	return site, nodes, log
}

func index(t *testing.T, log []string, line string) int {
	t.Helper()
	for i, l := range log {
		if l == line {
			return i
		}
	}
	t.Fatalf("%q never ran:\n%s", line, strings.Join(log, "\n"))
	return -1
}

func TestSiteStartsTheFirstControllerThenJoinsTheRestInOrder(t *testing.T) {
	site, nodes, log := newSite("v1.36.4+k0s.1")
	if err := site.Build(context.Background()); err != nil {
		t.Fatal(err)
	}
	for name, n := range nodes {
		if n.files["/usr/local/bin/k0s"] != "k0s-binary" {
			t.Errorf("%s did not receive the pinned binary", name)
		}
	}
	if !strings.Contains(nodes["cp2"].files["/etc/k0s/k0s.yaml"], "192.0.2.11") {
		t.Error("cp2's own native config was not written")
	}
	if _, ok := nodes["w1"].files["/etc/k0s/k0s.yaml"]; ok {
		t.Error("a worker received a controller config")
	}
	if nodes["cp2"].files["/etc/k0s/token"] != "token-controller" || nodes["w1"].files["/etc/k0s/token"] != "token-worker" {
		t.Errorf("tokens = %q, %q; want role-specific tokens without line endings",
			nodes["cp2"].files["/etc/k0s/token"], nodes["w1"].files["/etc/k0s/token"])
	}
	l := *log
	order := []string{
		"w1: k0s version",
		"cp1: k0s install controller --enable-worker --config=/etc/k0s/k0s.yaml",
		"cp1: k0s start",
		"cp1: k0s kubeconfig admin",
		"cp1: k0s token create --role controller",
		"cp2: k0s install controller --token-file=/etc/k0s/token --enable-worker --config=/etc/k0s/k0s.yaml",
		"cp2: k0s kubeconfig admin",
		"cp1: k0s token create --role worker",
		"w1: k0s install worker --token-file=/etc/k0s/token --kubelet-extra-args=--node-ip=192.0.2.20",
		"w1: k0s start",
	}
	last := -1
	for _, line := range order {
		i := index(t, l, line)
		if i < last {
			t.Fatalf("%q ran out of order:\n%s", line, strings.Join(l, "\n"))
		}
		last = i
	}
}

// Every member is checked before any is installed: a site of mixed
// versions is two incomparable sites.
func TestSiteRefusesAVersionMismatchBeforeInstalling(t *testing.T) {
	site, _, log := newSite("v1.35.0+k0s.0")
	err := site.Build(context.Background())
	if err == nil || !strings.Contains(err.Error(), "v1.35.0+k0s.0") {
		t.Fatalf("Build = %v, want the mismatched version named", err)
	}
	for _, line := range *log {
		if strings.Contains(line, "k0s install") {
			t.Fatalf("installed after a version mismatch: %s", line)
		}
	}
}

// An installed member is started again, not reinstalled or rejoined.
func TestSiteResumesInstalledMembers(t *testing.T) {
	site, nodes, log := newSite("v1.36.4+k0s.1", "cp1", "cp2", "w1")
	if err := site.Build(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, line := range *log {
		if strings.Contains(line, "k0s install") || strings.Contains(line, "token create") {
			t.Fatalf("an installed member was reinstalled: %s", line)
		}
	}
	if _, ok := nodes["w1"].files["/etc/k0s/token"]; ok {
		t.Fatal("an installed worker received a new join token")
	}
}
