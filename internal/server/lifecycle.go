package server

import (
	"context"

	"github.com/appmana/labcontainers/internal/session"
)

func (s *Server) verifyRunningNode(ctx context.Context, r *session.Record, name string) error {
	if _, err := s.Backend.ContainerName(ctx, r.Name, name); err != nil {
		return err
	}
	prepared, err := prepareDraft(r, nil)
	if err != nil {
		return err
	}
	for _, isolated := range prepared.IsolatedNodes {
		if isolated == name {
			return s.Backend.ProofIsolation(ctx, r.Name, []string{name})
		}
	}
	return nil
}
