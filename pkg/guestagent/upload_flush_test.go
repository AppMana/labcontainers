package guestagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Windows QGA CloseHandle does not flush cached file data. A successful Put
// must acknowledge guest-file-flush before closing, and cannot hide either
// error when a caller is about to power off the VM.
func TestUploadFlushesAndReportsFinalizationErrors(t *testing.T) {
	for _, failure := range []string{"", "guest-file-flush", "guest-file-close"} {
		t.Run(failure, func(t *testing.T) {
			socket := filepath.Join(t.TempDir(), "qga.sock")
			listener, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan []string, 1)
			go func() {
				var commands []string
				defer func() { done <- commands }()
				for {
					server, err := listener.Accept()
					if err != nil {
						return
					}
					defer server.Close()
					reader, encoder := bufio.NewReader(server), json.NewEncoder(server)
					for {
						var request struct {
							Execute   string         `json:"execute"`
							Arguments map[string]any `json:"arguments"`
						}
						line, err := reader.ReadBytes('\n')
						if err != nil {
							break
						}
						if json.Unmarshal(bytes.TrimLeft(line, "\xff"), &request) != nil {
							return
						}
						if request.Execute == "guest-sync-delimited" {
							if encoder.Encode(map[string]any{"return": request.Arguments["id"]}) != nil {
								return
							}
							continue
						}
						commands = append(commands, request.Execute)
						var reply any = map[string]any{}
						switch request.Execute {
						case "guest-file-open":
							reply = 7
						case "guest-file-write":
							reply = map[string]any{"count": 4}
						}
						response := map[string]any{"return": reply}
						if request.Execute == failure {
							response = map[string]any{"error": map[string]any{"desc": "injected finalization failure"}}
						}
						if encoder.Encode(response) != nil {
							return
						}
						if request.Execute == "guest-file-close" {
							return
						}
					}
				}
			}()
			agent := &Agent{Socket: socket}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err = agent.Upload(ctx, strings.NewReader("data"), `C:\lab\config`)
			want := []string{"guest-file-open", "guest-file-write", "guest-file-flush", "guest-file-close"}
			if got := <-done; !reflect.DeepEqual(got, want) {
				t.Errorf("commands=%v want=%v", got, want)
			}
			if failure == "" && err != nil {
				t.Fatal(err)
			}
			if failure != "" && (err == nil || !strings.Contains(err.Error(), failure)) {
				t.Fatalf("lost %s failure: %v", failure, err)
			}
		})
	}
}
