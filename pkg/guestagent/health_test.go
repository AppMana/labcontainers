package guestagent

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthUsesExistingQGAOwner(t *testing.T) {
	client, guest := net.Pipe()
	defer client.Close()
	defer guest.Close()
	_ = guest.SetDeadline(time.Now().Add(time.Second))
	done := make(chan string, 1)
	go func() {
		var request struct {
			Execute string `json:"execute"`
		}
		if err := json.NewDecoder(guest).Decode(&request); err != nil {
			done <- err.Error()
			return
		}
		_ = json.NewEncoder(guest).Encode(map[string]any{"return": map[string]any{}})
		done <- request.Execute
	}()
	owner := &Agent{conn: client, reader: bufio.NewReader(client), Socket: "/must-not-open-a-second-qga-connection"}
	req := httptest.NewRequest(http.MethodPost, "/ping", nil)
	req.Header.Set("X-Timeout", "500ms")
	response := httptest.NewRecorder()
	(&Server{Agent: owner}).ServeHTTP(response, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("health did not use shared QGA owner: status=%d body=%s", response.Code, response.Body.String())
	}
	if command := <-done; command != "guest-ping" {
		t.Fatalf("health executed %q instead of guest-ping", command)
	}
}

func TestHealthRejectsInvalidRequestsBeforeGuestAccess(t *testing.T) {
	for _, tc := range []struct{ method, timeout string }{
		{http.MethodGet, "1s"}, {http.MethodPost, ""}, {http.MethodPost, "0s"}, {http.MethodPost, "-1s"}, {http.MethodPost, "2m"},
	} {
		req := httptest.NewRequest(tc.method, "/ping", nil)
		req.Header.Set("X-Timeout", tc.timeout)
		response := httptest.NewRecorder()
		(&Server{}).ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid request accepted: %+v status=%d", tc, response.Code)
		}
	}
}

func TestHealthTimeoutDoesNotCloseAnotherCallsConnection(t *testing.T) {
	client, guest := net.Pipe()
	defer client.Close()
	defer guest.Close()
	owner := &Agent{conn: client}
	owner.once.Do(func() { owner.gate = make(chan struct{}, 1) })
	owner.gate <- struct{}{} // Another in-progress QGA command owns the connection.
	req := httptest.NewRequest(http.MethodPost, "/ping", nil)
	req.Header.Set("X-Timeout", "5ms")
	response := httptest.NewRecorder()
	(&Server{Agent: owner}).ServeHTTP(response, req)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatal("queued health probe did not honor its deadline", response.Code)
	}
	if err := client.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal("health timeout closed the execution owner's connection", err)
	}
}
