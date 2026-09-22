package client

import (
	"path/filepath"
	"testing"
)

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
