// Package client provides the Go Testcontainers-style Labcontainers SDK.
package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Options struct {
	Socket   string
	StateDir string
	LabdPath string
}

type Client struct {
	conn     *grpc.ClientConn
	rpc      labv1.LabcontainersClient
	cmd      *exec.Cmd
	tempDir  string
	mu       sync.Mutex
	sessions map[string]string
}

// Launch starts a private labd child and connects to it.
func Launch(ctx context.Context, opts Options) (*Client, error) {
	tempDir := ""
	if opts.Socket == "" {
		var err error
		tempDir, err = os.MkdirTemp("", "labcontainers-")
		if err != nil {
			return nil, err
		}
		opts.Socket = filepath.Join(tempDir, "labd.sock")
	}
	opts.StateDir = launchStateDir(tempDir, opts.StateDir)
	labd := opts.LabdPath
	if labd == "" {
		labd = "labd"
	}
	args := []string{"--socket", opts.Socket, "--parent-pid", fmt.Sprint(os.Getpid())}
	if opts.StateDir != "" {
		args = append(args, "--state-dir", opts.StateDir)
	}
	cmd := exec.CommandContext(context.Background(), labd, args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
		return nil, err
	}
	c, err := dialWhenReady(ctx, opts.Socket)
	if err != nil {
		_ = cmd.Process.Kill()
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
		return nil, err
	}
	c.cmd, c.tempDir = cmd, tempDir
	return c, nil
}

// Dial connects to an already running private daemon.
func launchStateDir(tempDir, configured string) string {
	if configured != "" || tempDir == "" {
		return configured
	}
	return filepath.Join(tempDir, "state")
}

func Dial(ctx context.Context, socket string) (*Client, error) {
	conn, err := grpc.DialContext(ctx, "unix://"+socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		}),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallSendMsgSize(labv1.MaxMessageBytes),
			grpc.MaxCallRecvMsgSize(labv1.MaxMessageBytes)))
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, rpc: labv1.NewLabcontainersClient(conn), sessions: map[string]string{}}, nil
}

func dialWhenReady(ctx context.Context, socket string) (*Client, error) {
	deadline := time.Now().Add(10 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		attempt, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		c, err := Dial(attempt, socket)
		cancel()
		if err == nil {
			return c, nil
		}
		last = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("connect to labd: %w", last)
}

func (c *Client) Start(ctx context.Context, spec *labv1.LabSpec, ttl time.Duration) (*Session, error) {
	p, err := c.rpc.CreateSession(ctx, &labv1.CreateSessionRequest{Spec: spec, TtlSeconds: int64(ttl / time.Second)})
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.sessions[p.GetId()] = p.GetResumeToken()
	c.mu.Unlock()
	return &Session{client: c, value: p}, nil
}

func (c *Client) Resume(ctx context.Context, id string) (*Session, error) {
	p, err := c.rpc.GetSession(ctx, &labv1.SessionRef{Id: id})
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.sessions[p.GetId()] = p.GetResumeToken()
	c.mu.Unlock()
	return &Session{client: c, value: p}, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	sessions := c.sessions
	c.sessions = map[string]string{}
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var first error
	for id, token := range sessions {
		if _, err := c.rpc.DestroySession(ctx, &labv1.DestroySessionRequest{Id: id, ResumeToken: token}); err != nil && first == nil {
			first = err
		}
	}
	if err := c.conn.Close(); err != nil && first == nil {
		first = err
	}
	if c.cmd != nil {
		_ = c.cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- c.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = c.cmd.Process.Kill()
			<-done
		}
	}
	if c.tempDir != "" {
		_ = os.RemoveAll(c.tempDir)
	}
	return first
}

type Session struct {
	client *Client
	value  *labv1.Session
}

func (s *Session) ID() string             { return s.value.GetId() }
func (s *Session) Name() string           { return s.value.GetName() }
func (s *Session) Artifacts() string      { return s.value.GetArtifactDirectory() }
func (s *Session) Node(name string) *Node { return &Node{session: s, name: name} }

func (s *Session) Keep(ctx context.Context, ttl time.Duration) error {
	p, err := s.client.rpc.KeepSession(ctx, &labv1.KeepSessionRequest{Id: s.ID(), TtlSeconds: int64(ttl / time.Second)})
	if err != nil {
		return err
	}
	s.value = p
	s.client.mu.Lock()
	delete(s.client.sessions, s.ID())
	s.client.mu.Unlock()
	return nil
}

func (s *Session) Destroy(ctx context.Context) error {
	_, err := s.client.rpc.DestroySession(ctx, &labv1.DestroySessionRequest{Id: s.ID(), ResumeToken: s.value.GetResumeToken()})
	if err == nil {
		s.client.mu.Lock()
		delete(s.client.sessions, s.ID())
		s.client.mu.Unlock()
	}
	return err
}

func (s *Session) Netem(ctx context.Context, node, iface string, impairment *labv1.Netem) (*Fault, error) {
	f, err := s.client.rpc.ApplyFault(ctx, &labv1.ApplyFaultRequest{SessionId: s.ID(), Node: node, Interface: iface, Fault: &labv1.ApplyFaultRequest_Netem{Netem: impairment}})
	if err != nil {
		return nil, err
	}
	return &Fault{session: s, value: f}, nil
}

func (s *Session) SetLink(ctx context.Context, node, iface string, up bool) (*Fault, error) {
	f, err := s.client.rpc.ApplyFault(ctx, &labv1.ApplyFaultRequest{SessionId: s.ID(), Fault: &labv1.ApplyFaultRequest_LinkState{LinkState: &labv1.LinkState{Node: node, Interface: iface, Up: up}}})
	if err != nil {
		return nil, err
	}
	return &Fault{session: s, value: f}, nil
}

func (s *Session) RunTimeline(ctx context.Context, actions ...*labv1.TimelineAction) (*labv1.TimelineResult, error) {
	return s.client.rpc.RunTimeline(ctx, &labv1.RunTimelineRequest{SessionId: s.ID(), Actions: actions})
}

type Fault struct {
	session *Session
	value   *labv1.Fault
}

func (f *Fault) ID() string { return f.value.GetId() }
func (f *Fault) Revert(ctx context.Context) error {
	_, err := f.session.client.rpc.RevertFault(ctx, &labv1.FaultRef{SessionId: f.session.ID(), Id: f.ID()})
	return err
}

type Node struct {
	session *Session
	name    string
}

func (n *Node) Exec(ctx context.Context, argv ...string) (*labv1.ExecResponse, error) {
	return n.session.client.rpc.Exec(ctx, &labv1.ExecRequest{Node: n.ref(), Argv: argv})
}

// ExecWithTimeout sets the guest command deadline instead of Exec's two-minute
// daemon default. The caller's context can still cancel the request earlier.
func (n *Node) ExecWithTimeout(ctx context.Context, timeout time.Duration, argv ...string) (*labv1.ExecResponse, error) {
	if timeout < time.Millisecond {
		return nil, fmt.Errorf("exec timeout must be at least one millisecond")
	}
	return n.session.client.rpc.Exec(ctx, &labv1.ExecRequest{Node: n.ref(), Argv: argv, TimeoutMillis: timeout.Milliseconds()})
}

func (n *Node) Put(ctx context.Context, path string, mode uint32, content []byte) error {
	_, err := n.session.client.rpc.Put(ctx, &labv1.PutRequest{Node: n.ref(), Path: path, Mode: mode, Content: content})
	return err
}

// Crash abruptly kills the node without guest shutdown. Attached disks persist.
func (n *Node) Crash(ctx context.Context) error { return n.lifecycle(ctx, labv1.LifecycleAction_CRASH) }

func (n *Node) PowerOff(ctx context.Context) error {
	return n.lifecycle(ctx, labv1.LifecycleAction_POWER_OFF)
}
func (n *Node) Start(ctx context.Context) error { return n.lifecycle(ctx, labv1.LifecycleAction_START) }
func (n *Node) Restart(ctx context.Context) error {
	return n.lifecycle(ctx, labv1.LifecycleAction_RESTART)
}
func (n *Node) Replace(ctx context.Context) error {
	return n.lifecycle(ctx, labv1.LifecycleAction_REPLACE)
}

// ReplaceWithBootstrap recreates the node and supplies new first-boot data.
func (n *Node) ReplaceWithBootstrap(ctx context.Context, data *labv1.BootstrapData) error {
	_, err := n.session.client.rpc.Lifecycle(ctx, &labv1.LifecycleRequest{Node: n.ref(), Action: labv1.LifecycleAction_REPLACE, Bootstrap: data})
	return err
}

func (n *Node) lifecycle(ctx context.Context, action labv1.LifecycleAction) error {
	_, err := n.session.client.rpc.Lifecycle(ctx, &labv1.LifecycleRequest{Node: n.ref(), Action: action})
	return err
}

func (n *Node) ref() *labv1.NodeRef { return &labv1.NodeRef{SessionId: n.session.ID(), Node: n.name} }
