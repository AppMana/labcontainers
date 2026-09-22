package guestagent

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/appmana/labcontainers/internal/transferlimits"
)

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestExecRejectsOversizedUnknownLengthInputBeforeGuestAccess(t *testing.T) {
	req := httptest.NewRequest("POST", "/exec", io.LimitReader(zeroReader{}, transferlimits.GuestExecStdin+1))
	req.Header.Set("X-Argv", base64.StdEncoding.EncodeToString([]byte(`["true"]`)))
	req.Header.Set("X-Timeout", "1s")
	req.Header.Set("X-Stdin", "1")
	response := httptest.NewRecorder()
	// A nil agent proves oversized stdin is rejected before executing anything.
	(&Server{}).ServeHTTP(response, req)
	var result Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != 125 || !strings.Contains(result.Error, "request body too large") {
		t.Fatalf("oversized input appeared successful: %+v", result)
	}
}

func TestPutRejectsKnownOversizeBeforeGuestAccess(t *testing.T) {
	req := httptest.NewRequest("POST", "/put", nil)
	req.ContentLength = transferlimits.Upload + 1
	response := httptest.NewRecorder()
	(&Server{}).ServeHTTP(response, req)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d", response.Code)
	}
}

func TestUploadDistinguishesExactBoundaryFromTruncation(t *testing.T) {
	for _, body := range []string{"four", "extra"} {
		t.Run(body, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()
			done := make(chan error, 1)
			go func() {
				decoder, encoder := json.NewDecoder(server), json.NewEncoder(server)
				for {
					var request struct {
						Execute   string         `json:"execute"`
						Arguments map[string]any `json:"arguments"`
					}
					if err := decoder.Decode(&request); err != nil {
						done <- err
						return
					}
					var reply any
					switch request.Execute {
					case "guest-file-open":
						reply = 1
					case "guest-file-write":
						data, err := base64.StdEncoding.DecodeString(request.Arguments["buf-b64"].(string))
						if err != nil {
							done <- err
							return
						}
						reply = map[string]any{"count": len(data)}
					case "guest-file-close":
						reply = map[string]any{}
					default:
						done <- fmt.Errorf("unexpected command %s", request.Execute)
						return
					}
					if err := encoder.Encode(map[string]any{"return": reply}); err != nil {
						done <- err
						return
					}
					if request.Execute == "guest-file-close" {
						done <- nil
						return
					}
				}
			}()
			agent := &Agent{conn: client, reader: bufio.NewReader(client)}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			limited := http.MaxBytesReader(httptest.NewRecorder(), io.NopCloser(strings.NewReader(body)), 4)
			err := agent.Upload(ctx, limited, "/tmp/exact")
			var tooLarge *http.MaxBytesError
			if len(body) == 4 && err != nil {
				t.Fatal(err)
			}
			if len(body) > 4 && !errors.As(err, &tooLarge) {
				t.Fatalf("expected size error, got %v", err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
