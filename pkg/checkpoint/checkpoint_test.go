package checkpoint

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestMarksCompleteness(t *testing.T) {
	dir := t.TempDir()
	if _, err := Read(dir); !errors.Is(err, ErrNone) {
		t.Fatalf("empty directory: %v", err)
	}
	m := Manifest{Session: "s", Nodes: map[string]Node{
		"vm": {Image: "sha256:abc", Root: []string{"base-overlay.qcow2"}, Disks: []Disk{{Name: "data", SizeBytes: 4}}},
	}}
	if err := Write(dir, m); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err == nil {
		t.Fatal("accepted a manifest whose files are missing")
	}
	for path, body := range map[string]string{RootPath(dir, "vm", "base-overlay.qcow2"): "overlay", DiskPath(dir, "vm", "data"): "disk"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != version || got.Nodes["vm"].Image != "sha256:abc" || len(got.NodeNames()) != 1 {
		t.Fatalf("unexpected manifest: %+v", got)
	}
	if err := os.WriteFile(DiskPath(dir, "vm", "data"), []byte("resized"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err == nil {
		t.Fatal("accepted a data disk whose size changed")
	}
	if size, err := Size(dir); err != nil || size == 0 {
		t.Fatalf("size: %d %v", size, err)
	}
}

func TestReadRejectsUnknownVersionAndMissingImage(t *testing.T) {
	for _, body := range []string{`{"version":2,"nodes":{"vm":{"image":"i"}}}`, `{"version":1,"nodes":{}}`, `{"version":1,"nodes":{"vm":{}}}`, `not json`} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(dir); err == nil || errors.Is(err, ErrNone) {
			t.Fatalf("accepted %s: %v", body, err)
		}
	}
}
