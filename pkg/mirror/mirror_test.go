package mirror

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestInstallerURLsResolveToStagedFilesOnly(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	staged := filepath.Join(root, "downloads.example.test", "v1.2.3", "bin", "tool.exe")
	if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("pinned bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("outside any host"), 0o644); err != nil {
		t.Fatal(err)
	}
	authority, err := LoadAuthority(state, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	again, err := LoadAuthority(state, time.Now())
	if err != nil || !again.Certificate.Equal(authority.Certificate) {
		t.Fatal("a restarted mirror must present the authority guests already trust")
	}
	tlsListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	crlListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	crlURL := "http://" + crlListener.Addr().String() + "/mirror.crl"
	log := &syncBuffer{}
	go func() { _ = Serve(root, authority, crlURL, tlsListener, crlListener, log) }()
	defer tlsListener.Close()
	defer crlListener.Close()

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(authority.PEM())
	var leaf *x509.Certificate
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		// The lab's name resolution: every staged host is the mirror.
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, tlsListener.Addr().String())
		},
		TLSClientConfig: &tls.Config{RootCAs: pool, VerifyConnection: func(state tls.ConnectionState) error {
			leaf = state.PeerCertificates[0]
			return nil
		}},
	}}
	get := func(url string) (int, string) {
		t.Helper()
		response, err := client.Get(url)
		if err != nil {
			t.Fatalf("%s: %v", url, err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(body)
	}
	if code, body := get("https://downloads.example.test/v1.2.3/bin/tool.exe"); code != 200 || body != "pinned bytes" {
		t.Fatalf("staged file: %d %q", code, body)
	}
	for _, url := range []string{
		"https://downloads.example.test/v9.9.9/bin/tool.exe",
		"https://downloads.example.test/../secret.txt",
		"https://downloads.example.test/%2e%2e/secret.txt",
		"https://downloads.example.test/v1.2.3/bin",
	} {
		if code, body := get(url); code != 404 || strings.Contains(body, "outside") {
			t.Fatalf("%s: %d %q", url, code, body)
		}
	}
	if _, err := client.Get("https://unstaged.example.test/"); err == nil {
		t.Fatal("served a host that was never staged under a certificate that names it")
	}
	if leaf == nil || len(leaf.CRLDistributionPoints) != 1 || leaf.CRLDistributionPoints[0] != crlURL {
		t.Fatalf("serving certificate names no revocation list: %+v", leaf)
	}
	response, err := http.Get(crlURL)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := io.ReadAll(response.Body)
	response.Body.Close()
	list, err := x509.ParseRevocationList(der)
	if err != nil {
		t.Fatal(err)
	}
	if err := list.CheckSignatureFrom(authority.Certificate); err != nil || len(list.RevokedCertificateEntries) != 0 || !list.NextUpdate.After(time.Now()) {
		t.Fatalf("revocation list: %v %+v", err, list)
	}
	var first Request
	if err := json.Unmarshal([]byte(strings.SplitN(log.String(), "\n", 2)[0]), &first); err != nil || first.Host != "downloads.example.test" || first.Status != 200 || first.Bytes != 12 {
		t.Fatalf("access log: %+v %v", first, err)
	}
	if !strings.Contains(log.String(), `"status":404`) {
		t.Fatal("unstaged requests must be logged")
	}
}

func TestHostsAreTheStagedDirectories(t *testing.T) {
	root := t.TempDir()
	if _, err := Hosts(root); err == nil {
		t.Fatal("an empty mirror started")
	}
	for _, dir := range []string{"b.example.test", "a.example.test", "notahost"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	hosts, err := Hosts(root)
	if err != nil || strings.Join(hosts, ",") != "a.example.test,b.example.test" {
		t.Fatalf("%v %v", hosts, err)
	}
}

func TestAuthorityRefusesPartialOrExpiredState(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadAuthority(dir, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuthority(dir, time.Now().Add(120*24*time.Hour)); err == nil {
		t.Fatal("used an expired authority")
	}
	if err := os.Remove(filepath.Join(dir, "ca.key")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuthority(dir, time.Now()); err == nil {
		t.Fatal("replaced half of an authority guests may already trust")
	}
}
