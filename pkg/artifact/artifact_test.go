package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPreparedArtifact(t *testing.T) {
	body := []byte{0, 255, 10, 0, 1}
	filename := filepath.Join(t.TempDir(), "prepared")
	if err := os.WriteFile(filename, body, 0o600); err != nil {
		t.Fatal(err)
	}
	pin := fmt.Sprintf("%x", sha256.Sum256(body))
	for _, digest := range []string{pin, strings.ToUpper(pin)} {
		got, err := ReadFile(context.Background(), filename, digest)
		if err != nil || !bytes.Equal(got, body) {
			t.Fatalf("payload changed: %x %v", got, err)
		}
	}
	for _, tc := range []struct{ path, pin string }{
		{"", pin}, {filename, ""}, {filename, "bad"},
		{filename, strings.Repeat("0", 64)}, {filename + "-missing", pin},
		{filepath.Dir(filename), pin},
	} {
		if got, err := ReadFile(context.Background(), tc.path, tc.pin); err == nil || got != nil {
			t.Fatalf("invalid artifact returned bytes: %q %x %v", tc.path, got, err)
		}
	}
	if err := os.WriteFile(filename, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	emptyPin := fmt.Sprintf("%x", sha256.Sum256(nil))
	if _, err := ReadFile(context.Background(), filename, emptyPin); err == nil {
		t.Fatal("accepted empty artifact")
	}
}

func TestCanceledRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadFile(ctx, "", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}
