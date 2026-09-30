package guestagent

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Health shares the same Agent and serialized connection as execution. QEMU's
// serial socket has one active client: a separate launcher connection can sit
// behind the persistent helper connection indefinitely.
func (s *Server) servePing(w http.ResponseWriter, req *http.Request) {
	duration, err := time.ParseDuration(req.Header.Get("X-Timeout"))
	if req.Method != http.MethodPost || err != nil || duration <= 0 || duration > time.Minute {
		http.Error(w, "invalid health request", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), duration)
	defer cancel()
	if err := s.Agent.Call(ctx, "guest-ping", nil, nil); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Ping checks the guest through the local helper, never opening QGA directly.
func Ping(ctx context.Context, execSocket string, timeout time.Duration) error {
	if execSocket == "" {
		execSocket = ExecSocket
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://guest/ping", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Timeout", timeout.String())
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", execSocket)
	}}
	defer transport.CloseIdleConnections()
	reply, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return err
	}
	defer reply.Body.Close()
	if reply.StatusCode != http.StatusNoContent {
		return fmt.Errorf("guest health status %d", reply.StatusCode)
	}
	return nil
}
