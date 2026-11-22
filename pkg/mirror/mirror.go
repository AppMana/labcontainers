// Package mirror serves pinned files at the URLs an installer downloads them
// from, inside a lab that has no route to those hosts. The installer runs
// unchanged: the lab resolves the original host names to the mirror and trusts
// the mirror's private authority. Nothing is fetched on demand; a URL that was
// not staged is a 404.
package mirror

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Authority is the private certificate authority guests are told to trust.
type Authority struct {
	Certificate *x509.Certificate
	key         *ecdsa.PrivateKey
}

// PEM is the certificate to install in a guest's trust store.
func (a *Authority) PEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: a.Certificate.Raw})
}

// LoadAuthority reads the authority kept in dir, creating it first if dir
// holds none. Keeping it lets a guest that already trusts the mirror keep
// doing so when the mirror restarts.
func LoadAuthority(dir string, now time.Time) (*Authority, error) {
	certPath, keyPath := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca.key")
	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	if os.IsNotExist(certErr) && os.IsNotExist(keyErr) {
		return createAuthority(dir, now)
	}
	if certErr != nil || keyErr != nil {
		return nil, fmt.Errorf("incomplete mirror authority in %s", dir)
	}
	certBlock, _ := pem.Decode(certPEM)
	keyBlock, _ := pem.Decode(keyPEM)
	if certBlock == nil || keyBlock == nil {
		return nil, fmt.Errorf("invalid mirror authority in %s", dir)
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}
	if now.After(cert.NotAfter) {
		return nil, fmt.Errorf("mirror authority in %s expired %s", dir, cert.NotAfter)
	}
	return &Authority{Certificate: cert, key: key}, nil
}

func createAuthority(dir string, now time.Time) (*Authority, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Labcontainers offline mirror"},
		// Guests are set from the host clock but may run slightly behind it.
		NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(90 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, MaxPathLenZero: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "ca.key"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return nil, err
	}
	a := &Authority{Certificate: cert, key: key}
	return a, os.WriteFile(filepath.Join(dir, "ca.pem"), a.PEM(), 0o644)
}

// Leaf issues the serving certificate for hosts. Clients that check
// revocation (Windows Schannel does, and fails closed without a distribution
// point) fetch the list from crlURL, which the mirror serves over plain HTTP.
func (a *Authority) Leaf(hosts []string, crlURL string, now time.Time) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return tls.Certificate{}, err
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: hosts[0]}, DNSNames: hosts,
		NotBefore: now.Add(-24 * time.Hour), NotAfter: a.Certificate.NotAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if crlURL != "" {
		template.CRLDistributionPoints = []string{crlURL}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, a.Certificate, &key.PublicKey, a.key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der, a.Certificate.Raw}, PrivateKey: key}, nil
}

// RevocationList is an empty list: nothing the mirror issues is revoked.
func (a *Authority) RevocationList(now time.Time) ([]byte, error) {
	return x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(now.Unix()), ThisUpdate: now.Add(-24 * time.Hour), NextUpdate: a.Certificate.NotAfter,
	}, a.Certificate, a.key)
}

// Hosts lists the host names staged under root: its top-level directories.
func Hosts(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var hosts []string
	for _, entry := range entries {
		if entry.IsDir() && strings.Contains(entry.Name(), ".") {
			hosts = append(hosts, entry.Name())
		}
	}
	sort.Strings(hosts)
	if len(hosts) == 0 {
		return nil, fmt.Errorf("%s stages no host directories", root)
	}
	return hosts, nil
}

// Request is one line of the mirror's access log: evidence of exactly what an
// installer asked for and whether it had been staged.
type Request struct {
	Time   time.Time `json:"time"`
	Client string    `json:"client"`
	Method string    `json:"method"`
	Host   string    `json:"host"`
	Path   string    `json:"path"`
	Status int       `json:"status"`
	Bytes  int64     `json:"bytes"`
}

// Handler serves root/<host>/<path> for requests to https://<host>/<path>.
type Handler struct {
	Root string
	Log  io.Writer
	mu   sync.Mutex
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	entry := Request{Time: time.Now().UTC(), Client: r.RemoteAddr, Method: r.Method, Host: r.Host, Path: r.URL.Path, Status: http.StatusOK}
	defer func() {
		if h.Log != nil {
			h.mu.Lock()
			defer h.mu.Unlock()
			_ = json.NewEncoder(h.Log).Encode(entry)
		}
	}()
	fail := func(code int) {
		entry.Status = code
		http.Error(w, http.StatusText(code), code)
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		fail(http.StatusMethodNotAllowed)
		return
	}
	host := r.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	clean := path.Clean("/" + r.URL.Path)
	if host == "" || strings.ContainsAny(host, `/\`) || host == "." || host == ".." {
		fail(http.StatusNotFound)
		return
	}
	file := filepath.Join(h.Root, host, filepath.FromSlash(clean))
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() {
		fail(http.StatusNotFound)
		return
	}
	f, err := os.Open(file)
	if err != nil {
		fail(http.StatusNotFound)
		return
	}
	defer f.Close()
	entry.Bytes = info.Size()
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// Serve answers HTTPS for every staged host on tlsListener and the revocation
// list on crlListener, until either fails.
func Serve(root string, authority *Authority, crlURL string, tlsListener, crlListener net.Listener, log io.Writer) error {
	hosts, err := Hosts(root)
	if err != nil {
		return err
	}
	now := time.Now()
	leaf, err := authority.Leaf(hosts, crlURL, now)
	if err != nil {
		return err
	}
	crl, err := authority.RevocationList(now)
	if err != nil {
		return err
	}
	failed := make(chan error, 2)
	go func() {
		failed <- http.Serve(crlListener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pkix-crl")
			_, _ = w.Write(crl)
		}))
	}()
	go func() {
		server := &http.Server{Handler: &Handler{Root: root, Log: log}}
		failed <- server.Serve(tls.NewListener(tlsListener, &tls.Config{Certificates: []tls.Certificate{leaf}, MinVersion: tls.VersionTLS12}))
	}()
	return <-failed
}
