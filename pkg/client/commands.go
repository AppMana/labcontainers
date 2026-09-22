package client

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/pkg/rig"
)

// Commands adapts this node to the older command/file contract used by the
// optional Kubernetes helpers. It adds no control network or lifecycle policy.
// Nonzero exits become rig.ExitError; stdout remains available with that error.
// Use Exec or the generated RPC directly when native response fields, explicit
// timeout values, or gRPC call options are needed. Pipe and Put buffer their
// readers because the underlying RPC carries bytes, not a stream.
func (n *Node) Commands() rig.Commands { return nodeCommands{node: n} }

type nodeCommands struct{ node *Node }

func (n nodeCommands) Name() string { return n.node.name }

func (n nodeCommands) Exec(ctx context.Context, argv ...string) ([]byte, error) {
	return n.Pipe(ctx, nil, argv...)
}

func (n nodeCommands) Pipe(ctx context.Context, src io.Reader, argv ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var stdin []byte
	if src != nil {
		var err error
		stdin, err = io.ReadAll(src)
		if err != nil {
			return nil, err
		}
	}
	response, err := n.node.session.client.rpc.Exec(ctx, &labv1.ExecRequest{Node: n.node.Ref(), Argv: argv, Stdin: stdin})
	if err != nil {
		return nil, err
	}
	if response.GetExitCode() != 0 {
		return response.GetStdout(), &rig.ExitError{Node: n.Name(), Argv: append([]string(nil), argv...), Code: int(response.GetExitCode()), Stderr: append([]byte(nil), response.GetStderr()...)}
	}
	return response.GetStdout(), nil
}

func (n nodeCommands) Put(ctx context.Context, src io.Reader, filename string, mode fs.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if src == nil {
		return fmt.Errorf("file content reader is required")
	}
	if mode != mode.Perm() {
		return fmt.Errorf("command adapter supports permission bits only; use native Put for other modes")
	}
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	return n.node.Put(ctx, filename, uint32(mode.Perm()), data)
}
