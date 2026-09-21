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

func TestExecutePreservesObservedResult(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	done := make(chan error, 1)
	go func() {
		decoder, encoder := json.NewDecoder(server), json.NewEncoder(server)
		for _, reply := range []any{map[string]any{"pid": 123}, map[string]any{"exited": true, "exitcode": 7, "out-data": base64.StdEncoding.EncodeToString([]byte("out")), "err-data": base64.StdEncoding.EncodeToString([]byte("err"))}} {
			var request map[string]any
			if err := decoder.Decode(&request); err != nil {
				done <- err
				return
			}
			if err := encoder.Encode(map[string]any{"return": reply}); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	a := &Agent{conn: client, reader: bufio.NewReader(client)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := a.Execute(ctx, []string{"true"})
	if err != nil || got.Code != 7 || string(got.Stdout) != "out" || string(got.Stderr) != "err" {
		t.Fatalf("result=%+v err=%v", got, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestReadSkipsStaleDelimitedData(t *testing.T) {
	a := &Agent{reader: bufio.NewReader(strings.NewReader("stale\n\xff{\"return\":123}\n"))}
	got, err := a.Read()
	if err != nil || strings.TrimSpace(string(got)) != `{"return":123}` {
		t.Fatalf("reply=%s err=%v", got, err)
	}
}
