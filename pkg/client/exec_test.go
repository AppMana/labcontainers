package client

import (
	"context"
	"reflect"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"google.golang.org/grpc"
)

type recordingExecClient struct {
	labv1.LabcontainersClient
	request *labv1.ExecRequest
	context context.Context
}

func (r *recordingExecClient) Exec(ctx context.Context, req *labv1.ExecRequest, _ ...grpc.CallOption) (*labv1.ExecResponse, error) {
	r.request, r.context = req, ctx
	return &labv1.ExecResponse{Stdout: []byte("done"), ExitCode: 7}, nil
}

func TestExecWithTimeout(t *testing.T) {
	rpc := &recordingExecClient{}
	n := (&Session{client: &Client{rpc: rpc}, value: &labv1.Session{Id: "private-session"}}).Node("windows")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	argv := []string{"powershell.exe", "-Command", "long-running test"}
	r, err := n.ExecWithTimeout(ctx, 10*time.Minute, argv...)
	if err != nil || string(r.GetStdout()) != "done" || r.GetExitCode() != 7 {
		t.Fatalf("response not preserved: %v, %v", r, err)
	}
	if rpc.request.GetTimeoutMillis() != 600000 || rpc.context != ctx ||
		rpc.request.GetNode().GetSessionId() != "private-session" || rpc.request.GetNode().GetNode() != "windows" ||
		!reflect.DeepEqual(rpc.request.GetArgv(), argv) {
		t.Fatalf("incorrect bounded request: %v", rpc.request)
	}
	for _, timeout := range []time.Duration{-time.Second, 0, time.Microsecond} {
		rpc.request = nil
		if _, err := n.ExecWithTimeout(ctx, timeout, argv...); err == nil || rpc.request != nil {
			t.Fatalf("invalid timeout %s reached daemon", timeout)
		}
	}
	if _, err := n.Exec(ctx, argv...); err != nil || rpc.request.GetTimeoutMillis() != 0 {
		t.Fatalf("default Exec behavior changed: %v", err)
	}
}
