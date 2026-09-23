package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
)

func TestEvidenceSurvivesEveryCleanupPath(t *testing.T) {
	for _, operation := range []string{"destroy", "owner-exit", "expire"} {
		t.Run(operation, func(t *testing.T) {
			s, _ := testServer(t)
			p := createTestSession(t, s)
			var err error
			event := "session.destroyed"
			switch operation {
			case "destroy":
				_, err = s.DestroySession(context.Background(), &labv1.DestroySessionRequest{Id: p.GetId()})
			case "owner-exit":
				err = s.CleanupUnkept(context.Background())
			case "expire":
				err = s.Scavenge(context.Background(), time.Now().Add(48*time.Hour))
				event = "session.expired"
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(s.Store.SessionDir(p.GetId())); !os.IsNotExist(err) {
				t.Fatalf("runtime state not removed: %v", err)
			}
			for _, file := range []string{"topology.clab.yml", "events.jsonl"} {
				content, err := os.ReadFile(filepath.Join(p.GetArtifactDirectory(), file))
				if err != nil || len(content) == 0 {
					t.Fatalf("lost retained %s: %v", file, err)
				}
				if file == "events.jsonl" && !strings.Contains(string(content), event) {
					t.Fatalf("missing final cleanup event: %s", content)
				}
			}
		})
	}
}

func TestEvidenceDirectoryPermissions(t *testing.T) {
	s, _ := testServer(t)
	p := createTestSession(t, s)
	for path, want := range map[string]os.FileMode{
		p.GetArtifactDirectory():                                     0o700,
		filepath.Join(p.GetArtifactDirectory(), "topology.clab.yml"): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("evidence permissions for %s: %v %v", path, info, err)
		}
	}
}
