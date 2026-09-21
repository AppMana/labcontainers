package client

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestPersistentReferenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "session.json")
	if err := writePersistentRef(path, persistentRef{ID: "session-id"}); err != nil {
		t.Fatal(err)
	}
	got, err := readPersistentRef(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "session-id" {
		t.Fatalf("id = %q", got.ID)
	}
}

func TestMissingPersistentReferenceIsDistinct(t *testing.T) {
	_, err := readPersistentRef(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, ErrNoPersistentSession) {
		t.Fatalf("error = %v", err)
	}
}
