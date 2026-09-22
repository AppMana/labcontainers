package client

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/pkg/rig"
	"google.golang.org/grpc"
)

type commandRPC struct {
	labv1.LabcontainersClient
	exec     *labv1.ExecRequest
	put      *labv1.PutRequest
	response *labv1.ExecResponse
	err      error
}

func (s *commandRPC) Exec(_ context.Context, request *labv1.ExecRequest, _ ...grpc.CallOption) (*labv1.ExecResponse, error) {
	s.exec = request
	return s.response, s.err
}
func (s *commandRPC) Put(_ context.Context, request *labv1.PutRequest, _ ...grpc.CallOption) (*labv1.Empty, error) {
	s.put = request
	return &labv1.Empty{}, s.err
}
func commandsFor(s *commandRPC) rig.Commands {
	session := &Session{client: &Client{rpc: s}, value: &labv1.Session{Id: "owned"}}
	return session.Node("guest").Commands()
}

func TestCommandsUsesExistingNativeRPC(t *testing.T) {
	s := &commandRPC{response: &labv1.ExecResponse{Stdout: []byte("result")}}
	n := commandsFor(s)
	args := []string{"tool", "spaces and ; $(no shell)"}
	out, err := n.Pipe(context.Background(), strings.NewReader("input\x00bytes"), args...)
	if err != nil || string(out) != "result" {
		t.Fatalf("result %q %v", out, err)
	}
	if s.exec.Node.SessionId != "owned" || s.exec.Node.Node != "guest" || !reflect.DeepEqual(s.exec.Argv, args) || string(s.exec.Stdin) != "input\x00bytes" || s.exec.TimeoutMillis != 0 {
		t.Fatalf("request changed: %v", s.exec)
	}
	if err := n.Put(context.Background(), strings.NewReader("file"), "/explicit/path", 0o600); err != nil {
		t.Fatal(err)
	}
	if s.put.Path != "/explicit/path" || s.put.Mode != 0o600 || string(s.put.Content) != "file" || s.put.Node.Node != "guest" {
		t.Fatalf("put changed: %v", s.put)
	}
}

func TestCommandsPreservesExitAndTransportErrors(t *testing.T) {
	s := &commandRPC{response: &labv1.ExecResponse{ExitCode: 7, Stdout: []byte("partial"), Stderr: []byte("diagnostic")}}
	argv := []string{"fails"}
	out, err := commandsFor(s).Exec(context.Background(), argv...)
	var exit *rig.ExitError
	if !errors.As(err, &exit) || exit.Code != 7 || exit.Node != "guest" || string(out) != "partial" || string(exit.Stderr) != "diagnostic" {
		t.Fatalf("lost exit details: %q %v", out, err)
	}
	argv[0] = "changed"
	if exit.Argv[0] != "fails" {
		t.Fatal("error aliases caller argv")
	}
	failure := errors.New("transport failed")
	s.err = failure
	if _, err := commandsFor(s).Exec(context.Background(), "tool"); !errors.Is(err, failure) {
		t.Fatal("lost transport error", err)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestCommandsRejectsInvalidInputsBeforeRPC(t *testing.T) {
	s := &commandRPC{}
	n := commandsFor(s)
	if _, err := n.Pipe(context.Background(), brokenReader{}, "tool"); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if err := n.Put(context.Background(), brokenReader{}, "/path", 0o600); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if err := n.Put(context.Background(), nil, "/path", 0o600); err == nil {
		t.Fatal("accepted nil reader")
	}
	if err := n.Put(context.Background(), strings.NewReader(""), "/path", fs.ModeSetuid|0o700); err == nil {
		t.Fatal("silently discarded special mode")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := n.Exec(ctx, "tool"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if s.exec != nil || s.put != nil {
		t.Fatal("invalid request reached RPC")
	}
}
