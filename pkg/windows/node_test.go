package windows

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	labv1 "github.com/appmana/labcontainers/api/v1"
)

type fakeGuest struct {
	log      []string
	respond  func(argv []string) *labv1.ExecResponse
	versions []string
}

func decode(argv []string) string {
	if len(argv) == 5 && argv[3] == "-EncodedCommand" {
		raw, _ := base64.StdEncoding.DecodeString(argv[4])
		words := make([]uint16, len(raw)/2)
		for i := range words {
			words[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
		}
		return string(utf16.Decode(words))
	}
	return strings.Join(argv, " ")
}

func (g *fakeGuest) ExecWithTimeout(_ context.Context, _ time.Duration, argv ...string) (*labv1.ExecResponse, error) {
	command := decode(argv)
	g.log = append(g.log, "exec "+command)
	if g.respond != nil {
		if r := g.respond(argv); r != nil {
			return r, nil
		}
	}
	if strings.Contains(command, "kubelet.exe --version") {
		version := g.versions[0]
		if len(g.versions) > 1 {
			g.versions = g.versions[1:]
		}
		return &labv1.ExecResponse{Stdout: []byte("Kubernetes v" + version + "\r\n")}, nil
	}
	return &labv1.ExecResponse{}, nil
}

func (g *fakeGuest) Put(_ context.Context, path string, _ uint32, content []byte) error {
	g.log = append(g.log, "put "+path+" "+string(content))
	return nil
}

func (g *fakeGuest) index(t *testing.T, fragment string) int {
	t.Helper()
	for i, entry := range g.log {
		if strings.Contains(entry, fragment) {
			return i
		}
	}
	t.Fatalf("no %q in:\n%s", fragment, strings.Join(g.log, "\n"))
	return -1
}

func TestKubeletUpgradeStopsBeforeReplacingAndVerifiesTheResult(t *testing.T) {
	guest := &fakeGuest{versions: []string{"1.37.2"}}
	kubelet := Kubelet{PrepareNode: []byte("PREPARE"), StartKubelet: []byte("START"), Version: "1.37.2"}
	if _, err := kubelet.Upgrade(context.Background(), guest); err != nil {
		t.Fatal(err)
	}
	stop, prepare := guest.index(t, "Stop-Service kubelet"), guest.index(t, `-File C:\PrepareNode.ps1 -KubernetesVersion v1.37.2 -ContainerRuntime ContainerD`)
	script, start := guest.index(t, `put C:\k\StartKubelet.ps1 START`), guest.index(t, "Start-Service kubelet")
	if !(stop < prepare && prepare < script && script < start) {
		t.Fatalf("wrong order:\n%s", strings.Join(guest.log, "\n"))
	}
	if strings.Contains(strings.Join(guest.log, "\n"), "bootstrap-kubeconfig.yaml PREPARE") || strings.Contains(strings.Join(guest.log, "\n"), `put C:\k\config`) {
		t.Fatal("an upgrade must not touch the node's credentials")
	}
	stale := &fakeGuest{versions: []string{"1.36.4"}}
	if _, err := kubelet.Upgrade(context.Background(), stale); err == nil || !strings.Contains(err.Error(), "1.36.4 after preparing 1.37.2") {
		t.Fatalf("accepted a node still on the old kubelet: %v", err)
	}
	failing := &fakeGuest{respond: func(argv []string) *labv1.ExecResponse {
		if strings.Contains(strings.Join(argv, " "), "PrepareNode.ps1") {
			return &labv1.ExecResponse{ExitCode: 1, Stderr: []byte("Download failed")}
		}
		return nil
	}}
	if _, err := kubelet.Upgrade(context.Background(), failing); err == nil || strings.Contains(strings.Join(failing.log, "\n"), "Start-Service") {
		t.Fatalf("started the kubelet after a failed preparation: %v", err)
	}
}

func TestKubeletJoinSeedsCredentialsOnlyOnAFreshNode(t *testing.T) {
	kubeconfig := []byte("clusters:\n- cluster:\n    certificate-authority-data: Q0E=\n")
	guest := &fakeGuest{versions: []string{"1.36.4"}}
	kubelet := Kubelet{PrepareNode: []byte("PREPARE"), StartKubelet: []byte("START"), Version: "1.36.4"}
	if _, err := kubelet.Join(context.Background(), guest, kubeconfig); err != nil {
		t.Fatal(err)
	}
	check, seed, prepare, ca, start := guest.index(t, "node already holds a bootstrap kubeconfig"), guest.index(t, `put C:\bootstrap-kubeconfig.yaml`), guest.index(t, "PrepareNode.ps1 -KubernetesVersion v1.36.4"), guest.index(t, `C:\var\lib\kubelet\pki\ca.crt`), guest.index(t, "Start-Service kubelet")
	if !(check < seed && seed < prepare && prepare < ca && ca < start) {
		t.Fatalf("wrong order:\n%s", strings.Join(guest.log, "\n"))
	}
	joined := &fakeGuest{respond: func(argv []string) *labv1.ExecResponse {
		if strings.Contains(decode(argv), "node already holds") {
			return &labv1.ExecResponse{ExitCode: 1, Stderr: []byte("node already holds a bootstrap kubeconfig")}
		}
		return nil
	}}
	if _, err := kubelet.Join(context.Background(), joined, kubeconfig); err == nil || len(joined.log) != 1 {
		t.Fatalf("rejoined an existing node: %v %v", err, joined.log)
	}
	if _, err := kubelet.Join(context.Background(), &fakeGuest{}, []byte("no authority")); err == nil {
		t.Fatal("accepted a kubeconfig without a certificate authority")
	}
	for _, bad := range []Kubelet{{StartKubelet: []byte("s"), Version: "1.36.4"}, {PrepareNode: []byte("p"), StartKubelet: []byte("s"), Version: "v1.36"}} {
		if _, err := bad.Upgrade(context.Background(), &fakeGuest{}); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}

func TestRuntimeTransactionPassesTheDeploymentsArguments(t *testing.T) {
	sha := strings.Repeat("a", 64)
	guest := &fakeGuest{respond: func(argv []string) *labv1.ExecResponse {
		line := strings.Join(argv, " ")
		switch {
		case strings.Contains(line, "-Mode Stage"):
			return &labv1.ExecResponse{Stdout: []byte(`{"stage":"C:\\releases\\2.3.6-aaaa","version":"2.3.6"}` + "\r\n")}
		case strings.Contains(line, "-Mode Apply"):
			return &labv1.ExecResponse{Stdout: []byte("CHANGED\r\n")}
		}
		return nil
	}}
	tx := RuntimeTransaction{Script: []byte("TX"), Path: `C:\deploy\transaction.ps1`}
	stage, err := tx.Stage(context.Background(), guest, `C:\deploy\runtime.tar.gz`, sha, "2.3.6-fork.post.1")
	if err != nil || stage != `C:\releases\2.3.6-aaaa` {
		t.Fatalf("%q %v", stage, err)
	}
	guest.index(t, `-File C:\deploy\transaction.ps1 -Mode Stage -Archive C:\deploy\runtime.tar.gz -ArchiveSHA256 `+sha+` -Version 2.3.6-fork.post.1`)
	changed, err := tx.Apply(context.Background(), guest, sha, "2.3.6-fork.post.1")
	if err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	guest.index(t, `-Mode Apply -ArchiveSHA256 `+sha+` -Version 2.3.6-fork.post.1`)
	refused := &fakeGuest{respond: func([]string) *labv1.ExecResponse {
		return &labv1.ExecResponse{ExitCode: 1, Stderr: []byte("Runtime downgrade refused")}
	}}
	if _, err := tx.Apply(context.Background(), refused, sha, "2.3.5"); err == nil || !strings.Contains(err.Error(), "downgrade refused") {
		t.Fatalf("lost the script's refusal: %v", err)
	}
	silent := &fakeGuest{}
	if _, err := tx.Apply(context.Background(), silent, sha, "2.3.6"); err == nil {
		t.Fatal("accepted an apply that reported no outcome")
	}
	if _, err := tx.Stage(context.Background(), guest, "a", "short", "2.3.6"); err == nil {
		t.Fatal("accepted an unpinned archive")
	}
}
