package guestagent

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

// QGA keeps exited process metadata until guest-exec-status reaps it. Windows
// can reuse the numeric PID meanwhile; QGA returns the first matching record.
// Model that protocol behavior, not a mock that silently discards old results.
func TestExecuteTimeoutCannotReturnStaleReusedPIDResult(t *testing.T) {
	for _, exitsBeforeCleanup := range []bool{true, false} {
		name := "kill-required"
		if exitsBeforeCleanup {
			name = "already-exited"
		}
		t.Run(name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type process struct {
				pid    int
				out    string
				exited bool
			}
			type observation struct{ remaining, kills int }
			observed := make(chan observation, 1)
			go func() {
				decoder, encoder := json.NewDecoder(server), json.NewEncoder(server)
				var records []process
				firstStatus, secondStarted, kills := true, false, 0
				for {
					var request struct {
						Execute   string `json:"execute"`
						Arguments struct {
							Path string `json:"path"`
							PID  int    `json:"pid"`
						} `json:"arguments"`
					}
					if decoder.Decode(&request) != nil {
						return
					}
					var reply any
					switch request.Execute {
					case "guest-exec":
						switch {
						case request.Arguments.Path == "probe-A":
							records = append(records, process{42, "windows-ready", false})
							reply = map[string]any{"pid": 42}
						case strings.HasSuffix(request.Arguments.Path, "taskkill.exe"):
							kills++
							for i := range records {
								if records[i].pid == 42 {
									records[i].exited = true
								}
							}
							records = append(records, process{43, "killed", true})
							reply = map[string]any{"pid": 43}
						case request.Arguments.Path == "command-B":
							secondStarted = true
							records = append(records, process{42, "setup-complete", true})
							reply = map[string]any{"pid": 42}
						default:
							return
						}
					case "guest-exec-status":
						index := -1
						for i := range records {
							if records[i].pid == request.Arguments.PID {
								index = i
								break
							}
						}
						if index < 0 {
							return
						}
						p := records[index]
						reply = map[string]any{"exited": p.exited, "exitcode": 0}
						if p.exited {
							reply.(map[string]any)["out-data"] = base64.StdEncoding.EncodeToString([]byte(p.out))
							records = append(records[:index], records[index+1:]...)
						}
						if firstStatus {
							firstStatus = false
							if exitsBeforeCleanup {
								records[index].exited = true
							}
							cancel() // cancellation after a running status, not a broken socket
						}
						if secondStarted && p.exited {
							observed <- observation{len(records), kills}
						}
					default:
						return
					}
					if encoder.Encode(map[string]any{"return": reply}) != nil {
						return
					}
				}
			}()
			a := &Agent{conn: client, reader: bufio.NewReader(client), os: "windows"}
			if _, err := a.Execute(ctx, []string{"probe-A"}, nil); err == nil {
				t.Fatal("cancelled command unexpectedly succeeded")
			}
			secondCtx, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			got, err := a.Execute(secondCtx, []string{"command-B"}, nil)
			if err != nil || string(got.Stdout) != "setup-complete" {
				t.Fatalf("reused PID returned result=%+v error=%v; must not return probe-A output", got, err)
			}
			select {
			case state := <-observed:
				if state.remaining != 0 {
					t.Fatalf("left %d unreaped process records", state.remaining)
				}
				if exitsBeforeCleanup && state.kills != 0 {
					t.Fatal("attempted to kill an already exited PID")
				}
			case <-secondCtx.Done():
				t.Fatal("missing protocol observation")
			}
		})
	}
}
