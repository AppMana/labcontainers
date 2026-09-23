package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupRetainsNonemptyOrUnreadableState(t *testing.T) {
	root := t.TempDir()
	if !retainState(root) || !retainState("") {
		t.Fatal("uncertain state considered disposable")
	}
	if err := os.Mkdir(filepath.Join(root, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if retainState(root) {
		t.Fatal("empty state retained")
	}
	if err := os.Mkdir(filepath.Join(root, "sessions", "kept"), 0o700); err != nil {
		t.Fatal(err)
	}
	if !retainState(root) {
		t.Fatal("kept state considered disposable")
	}
}

func TestLaunchStateDir(t *testing.T) {
	t.Parallel()

	private := t.TempDir()
	if got, want := launchStateDir(private, ""), filepath.Join(private, "state"); got != want {
		t.Fatalf("private daemon state directory = %q, want %q", got, want)
	}
	if got := launchStateDir(private, "/configured/state"); got != "/configured/state" {
		t.Fatalf("configured state directory changed to %q", got)
	}
	if got := launchStateDir("", ""); got != "" {
		t.Fatalf("caller-owned socket unexpectedly gained state directory %q", got)
	}
}
