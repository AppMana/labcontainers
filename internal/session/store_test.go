package session

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	want := &Record{ID: "one", Name: "lc-one", State: "running", Expires: time.Now().UTC().Truncate(time.Second), Nodes: map[string]*Node{"n1": {Name: "n1", State: "running"}}}
	if err := s.Create(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("one")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name || got.Nodes["n1"].State != "running" {
		t.Fatalf("got %#v", got)
	}
	if err := s.Delete("one"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("one"); err != ErrNotFound {
		t.Fatalf("got %v", err)
	}
}
