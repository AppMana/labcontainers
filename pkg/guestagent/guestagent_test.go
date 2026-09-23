package guestagent

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
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
		for index, reply := range []any{map[string]any{"pid": 123}, map[string]any{"exited": true, "exitcode": 7, "out-data": base64.StdEncoding.EncodeToString([]byte("out")), "err-data": base64.StdEncoding.EncodeToString([]byte("err"))}} {
			var request map[string]any
			if err := decoder.Decode(&request); err != nil {
				done <- err
				return
			}
			if index == 0 {
				arguments, _ := request["arguments"].(map[string]any)
				if arguments["input-data"] != base64.StdEncoding.EncodeToString([]byte("stdin")) {
					done <- errors.New("guest-exec did not preserve stdin as input-data")
					return
				}
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
	got, err := a.Execute(ctx, []string{"true"}, []byte("stdin"))
	if err != nil || got.Code != 7 || string(got.Stdout) != "out" || string(got.Stderr) != "err" {
		t.Fatalf("result=%+v err=%v", got, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCallCancellationInterruptsSocketAndQueue(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	a := &Agent{conn: client, reader: bufio.NewReader(client)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan error, 1)
	go func() { first <- a.Call(ctx, "guest-ping", nil, nil) }()
	// Observe the first request, then deliberately withhold its reply.
	if _, err := bufio.NewReader(server).ReadBytes('\n'); err != nil {
		t.Fatal(err)
	}
	queued, done := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer done()
	second := make(chan error, 1)
	go func() { second <- a.Call(queued, "guest-ping", nil, nil) }()
	select {
	case err := <-second:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("queued request: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued call ignored its deadline")
	}
	cancel()
	select {
	case err := <-first:
		if err == nil {
			t.Fatal("canceled socket returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("socket ignored cancellation")
	}
}

func TestReadSkipsStaleDelimitedData(t *testing.T) {
	a := &Agent{reader: bufio.NewReader(strings.NewReader("stale\n\xff{\"return\":123}\n"))}

	got, err := a.Read()
	if err != nil || strings.TrimSpace(string(got)) != `{"return":123}` {
		t.Fatalf("reply=%s err=%v", got, err)
	}
}

func TestMarshalCommandOmitsArgumentsForNoArgumentCommand(t *testing.T) {
	got, err := marshalCommand("guest-get-osinfo", nil)
	if err != nil {
		t.Fatal(err)
	}
	var request map[string]json.RawMessage
	if err := json.Unmarshal(got, &request); err != nil {
		t.Fatal(err)
	}
	if _, exists := request["arguments"]; exists {
		t.Fatalf("no-argument QGA request contains arguments: %s", got)
	}

	got, err = marshalCommand("guest-exec-status", map[string]any{"pid": 42})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &request); err != nil {
		t.Fatal(err)
	}
	if _, exists := request["arguments"]; !exists {
		t.Fatalf("argument-bearing QGA request omitted arguments: %s", got)
	}
}

func TestWaitForSocketHandlesLauncherRace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guest.sock")
	ready := make(chan net.Listener, 1)
	go func() {
		time.Sleep(25 * time.Millisecond)
		listener, err := net.Listen("unix", path)
		if err != nil {
			ready <- nil
			return
		}
		ready <- listener
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := waitForSocket(ctx, path); err != nil {
		t.Fatal(err)
	}
	listener := <-ready
	if listener == nil {
		t.Fatal("listener creation failed")
	}
	listener.Close()
}

func TestWindowsOSIdentifiers(t *testing.T) {
	for _, value := range []string{"windows", "mswindows", "Windows", "win32"} {
		if !isWindowsOS(value) {
			t.Errorf("%q not recognized as Windows", value)
		}
	}
	if isWindowsOS("linux") {
		t.Fatal("Linux recognized as Windows")
	}
}
