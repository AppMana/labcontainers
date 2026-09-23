package server

import (
	"fmt"
	"os"
	"path/filepath"
)

// Evidence is deliberately outside session state: destroying a lab (including
// its private daemon directory) must not destroy its diagnostic evidence.
func artifactDirectory(requested, id string) (string, error) {
	if requested != "" {
		return filepath.Abs(requested)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate retained evidence directory: %w", err)
	}
	return filepath.Join(cache, "labcontainers", "artifacts", id), nil
}
