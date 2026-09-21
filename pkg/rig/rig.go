// Package rig is the language-neutral behavioral contract implemented by lab nodes.
package rig

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/appmana/labcontainers/pkg/bootstrap"
)

type Node interface {
	Name() string
	Interface(nth int) string
	Exec(context.Context, ...string) ([]byte, error)
	Pipe(context.Context, io.Reader, ...string) ([]byte, error)
	Put(context.Context, io.Reader, string, fs.FileMode) error
	Cut(context.Context) error
	Restore(context.Context) error
	Kill(context.Context) error
	Boot(context.Context) error
	Userdata(context.Context, []byte) error
}

type Nodes interface{ Node(string) Node }

type Rig interface {
	Nodes
	Up(context.Context) error
	Down(context.Context) error
	Kind() string
}

type Fleet struct {
	Site    Nodes
	Remotes map[string]Node
}

func (f *Fleet) Node(name string) Node {
	if n, ok := f.Remotes[name]; ok {
		return n
	}
	return f.Site.Node(name)
}

type ExitError struct {
	Node   string
	Argv   []string
	Code   int
	Stderr []byte
}

func (e *ExitError) Error() string {
	msg := fmt.Sprintf("%s: %v exited %d", e.Node, e.Argv, e.Code)
	if len(e.Stderr) > 0 {
		msg += ": " + string(e.Stderr)
	}
	return msg
}

type BootstrapConsumer interface {
	Bootstrap(context.Context, bootstrap.Data) error
}
type InstanceBootstrapConsumer interface {
	BootstrapInstance(context.Context, string, bootstrap.Data) error
}

func BootstrapInstance(ctx context.Context, node Node, uid string, data bootstrap.Data) error {
	if uid == "" {
		return fmt.Errorf("infrastructure instance UID required")
	}
	if err := data.Validate(); err != nil {
		return err
	}
	if native, ok := node.(InstanceBootstrapConsumer); ok {
		return native.BootstrapInstance(ctx, uid, data)
	}
	return Bootstrap(ctx, node, data)
}

func Bootstrap(ctx context.Context, node Node, data bootstrap.Data) error {
	if err := data.Validate(); err != nil {
		return fmt.Errorf("%s: %w", node.Name(), err)
	}
	if native, ok := node.(BootstrapConsumer); ok {
		return native.Bootstrap(ctx, data)
	}
	if data.Format != bootstrap.CloudConfig {
		return fmt.Errorf("%s: machine does not support bootstrap format %q", node.Name(), data.Format)
	}
	return node.Userdata(ctx, data.Value)
}
