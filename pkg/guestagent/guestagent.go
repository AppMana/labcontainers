// Package guestagent carries command execution over QEMU Guest Agent's virtio-serial channel.
package guestagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/appmana/labcontainers/internal/transferlimits"
)

const (
	ExecSocket  = "/run/labcontainers-exec.sock"
	AgentSocket = "/run/labcontainers-qga.sock"
)

type Agent struct {
	mu     sync.Mutex
	once   sync.Once
	gate   chan struct{}
	conn   net.Conn
	reader *bufio.Reader
	Socket string
	os     string
}

// OS returns the guest operating-system identifier reported by QEMU Guest
// Agent (for example "linux" or "windows").
func (a *Agent) OS(ctx context.Context) (string, error) {
	a.mu.Lock()
	if a.os != "" {
		defer a.mu.Unlock()
		return a.os, nil
	}
	a.mu.Unlock()
	var info struct {
		ID string `json:"id"`
	}
	if err := a.Call(ctx, "guest-get-osinfo", nil, &info); err != nil {
		return "", err
	}
	a.mu.Lock()
	a.os = strings.ToLower(info.ID)
	a.mu.Unlock()
	return strings.ToLower(info.ID), nil
}

type Result struct {
	Stdout []byte `json:"stdout"`
	Stderr []byte `json:"stderr"`
	Code   int    `json:"code"`
	Error  string `json:"error,omitempty"`
}

func isWindowsOS(value string) bool {
	value = strings.ToLower(value)
	return value == "windows" || value == "mswindows" || strings.HasPrefix(value, "win")
}

func (a *Agent) socket() string {
	if a.Socket != "" {
		return a.Socket
	}
	return AgentSocket
}

func (a *Agent) Read() (json.RawMessage, error) {
	for {
		line, err := a.reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		line = bytes.TrimLeft(line, "\xff")
		if json.Valid(line) {
			return line, nil
		}
	}
}

func marshalCommand(command string, args any) ([]byte, error) {
	request := map[string]any{"execute": command}
	if args != nil {
		request["arguments"] = args
	}
	return json.Marshal(request)
}

func callDeadline(ctx context.Context, maximum time.Duration) time.Time {
	deadline := time.Now().Add(maximum)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		return value
	}
	return deadline
}

func dialUntilReady(ctx context.Context, network, address string) (net.Conn, error) {
	for {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err == nil {
			return conn, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (a *Agent) Call(ctx context.Context, command string, args, into any) (err error) {
	a.once.Do(func() { a.gate = make(chan struct{}, 1) })
	select {
	case a.gate <- struct{}{}:
		defer func() { <-a.gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if err != nil && a.conn != nil {
			_ = a.conn.Close()
			a.conn = nil
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	newConnection := a.conn == nil
	if newConnection {
		a.conn, err = dialUntilReady(ctx, "unix", a.socket())
		if err != nil {
			return err
		}
		a.reader = bufio.NewReader(a.conn)
	}
	// Socket deadlines alone do not react to cancellation before a deadline.
	// Stop the callback before releasing the gate to the next request.
	conn := a.conn
	interrupted := make(chan struct{})
	// Closing cannot be undone by a concurrent SetDeadline below.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close(); close(interrupted) })
	defer func() {
		if !stop() {
			<-interrupted
		}
	}()
	if newConnection {
		_ = a.conn.SetDeadline(callDeadline(ctx, 10*time.Minute))
		token := time.Now().UnixNano() & ((1 << 52) - 1)
		request, _ := marshalCommand("guest-sync-delimited", map[string]any{"id": token})
		if _, err = a.conn.Write(append(append([]byte{255}, request...), '\n')); err != nil {
			return err
		}
		for {
			raw, readErr := a.Read()
			if readErr != nil {
				return readErr
			}
			var reply struct {
				Return int64 `json:"return"`
			}
			if json.Unmarshal(raw, &reply) == nil && reply.Return == token {
				break
			}
		}
	}
	_ = a.conn.SetDeadline(callDeadline(ctx, 30*time.Second))
	request, err := marshalCommand(command, args)
	if err != nil {
		return err
	}
	if _, err = a.conn.Write(append(request, '\n')); err != nil {
		return err
	}
	raw, err := a.Read()
	if err != nil {
		return err
	}
	var reply struct {
		Return json.RawMessage `json:"return"`
		Error  *struct {
			Desc string `json:"desc"`
		} `json:"error"`
	}
	if err = json.Unmarshal(raw, &reply); err != nil {
		return err
	}
	if reply.Error != nil {
		return fmt.Errorf("%s: %s", command, reply.Error.Desc)
	}
	if into != nil {
		return json.Unmarshal(reply.Return, into)
	}
	return nil
}

func (a *Agent) Upload(ctx context.Context, body io.Reader, path string) error {
	var handle int64
	if err := a.Call(ctx, "guest-file-open", map[string]any{"path": path, "mode": "w"}, &handle); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = a.Call(cleanup, "guest-file-close", map[string]any{"handle": handle}, nil)
	}()
	buf := make([]byte, 192*1024)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			var written struct {
				Count int `json:"count"`
			}
			if e := a.Call(ctx, "guest-file-write", map[string]any{"handle": handle, "buf-b64": base64.StdEncoding.EncodeToString(buf[:n])}, &written); e != nil {
				return e
			}
			if written.Count != n {
				return io.ErrShortWrite
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// execExited consumes QGA's completed-process record, including captured output.
// Killing a process alone does not release that record: a reused Windows PID
// can otherwise make a later command receive the old command's result.
func (a *Agent) execExited(ctx context.Context, pid int) (bool, error) {
	var state struct {
		Exited bool `json:"exited"`
	}
	err := a.Call(ctx, "guest-exec-status", map[string]any{"pid": pid}, &state)
	return state.Exited, err
}

func (a *Agent) reapExec(ctx context.Context, pids ...int) error {
	pending := append([]int(nil), pids...)
	for len(pending) != 0 {
		next := pending[:0]
		for _, pid := range pending {
			exited, err := a.execExited(ctx, pid)
			if err != nil {
				return fmt.Errorf("reap PID %d: %w", pid, err)
			}
			if !exited {
				next = append(next, pid)
			}
		}
		pending = next
		if len(pending) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("unreaped PIDs %v: %w", pending, ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	return nil
}

func (a *Agent) cleanupExec(ctx context.Context, pid int) error {
	// Reap an already-exited command without targeting its possibly reused OS PID.
	exited, err := a.execExited(ctx, pid)
	if err != nil || exited {
		return err
	}
	guestOS, err := a.OS(ctx)
	if err != nil {
		return err
	}
	path, args := "kill", []string{"-TERM", strconv.Itoa(pid)}
	if isWindowsOS(guestOS) {
		path, args = `C:\Windows\System32\taskkill.exe`, []string{"/PID", strconv.Itoa(pid), "/T", "/F"}
	}
	var helper struct {
		PID int `json:"pid"`
	}
	if err := a.Call(ctx, "guest-exec", map[string]any{"path": path, "arg": args, "capture-output": false}, &helper); err != nil {
		return err
	}
	if helper.PID <= 0 {
		return fmt.Errorf("cleanup command returned no process ID")
	}
	// Poll the target first and interleave both records. A slow helper must not
	// consume the entire cleanup deadline while an exited target remains stale.
	// The helper owns a QGA record even without output capture.
	return a.reapExec(ctx, pid, helper.PID)
}

func (a *Agent) Execute(ctx context.Context, argv []string, input []byte) (result Result, resultErr error) {
	if len(argv) == 0 {
		return Result{}, fmt.Errorf("empty command")
	}
	var process struct {
		PID int `json:"pid"`
	}
	args := map[string]any{"path": argv[0], "arg": argv[1:], "capture-output": true}
	if input != nil {
		args["input-data"] = base64.StdEncoding.EncodeToString(input)
	}
	if err := a.Call(ctx, "guest-exec", args, &process); err != nil {
		return Result{}, err
	}
	if process.PID <= 0 {
		return Result{}, fmt.Errorf("guest agent returned no process ID")
	}
	exited := false
	defer func() {
		if exited {
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.cleanupExec(cleanup, process.PID); err != nil {
			resultErr = fmt.Errorf("%w; guest command cleanup: %v", resultErr, err)
		}
	}()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var state struct {
			Exited       bool   `json:"exited"`
			Code         int    `json:"exitcode"`
			Signal       int    `json:"signal"`
			Out          []byte `json:"out-data"`
			Err          []byte `json:"err-data"`
			OutTruncated bool   `json:"out-truncated"`
			ErrTruncated bool   `json:"err-truncated"`
		}
		if err := a.Call(ctx, "guest-exec-status", map[string]any{"pid": process.PID}, &state); err != nil {
			return Result{}, err
		}
		if state.Exited {
			exited = true
			if state.OutTruncated || state.ErrTruncated {
				return Result{}, fmt.Errorf("guest output was truncated")
			}
			if state.Signal != 0 {
				state.Code = 128 + state.Signal
			}
			return Result{Stdout: state.Out, Stderr: state.Err, Code: state.Code}, nil
		}
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

type Server struct{ Agent *Agent }

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/put" {
		s.servePut(w, req)
		return
	}
	var argv []string
	data, err := base64.StdEncoding.DecodeString(req.Header.Get("X-Argv"))
	if err == nil {
		err = json.Unmarshal(data, &argv)
	}
	duration, durationErr := time.ParseDuration(req.Header.Get("X-Timeout"))
	if err != nil || len(argv) == 0 || durationErr != nil || duration <= 0 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), duration)
	defer cancel()
	var result Result
	var input []byte
	if req.Header.Get("X-Stdin") == "1" {
		input, err = io.ReadAll(http.MaxBytesReader(w, req.Body, transferlimits.GuestExecStdin))
	}
	if err == nil {
		guestOS, osErr := s.Agent.OS(ctx)
		if osErr != nil {
			err = osErr
		} else {
			if !isWindowsOS(guestOS) {
				deadline, _ := ctx.Deadline()
				seconds := strconv.FormatFloat(time.Until(deadline).Seconds(), 'f', 3, 64)
				argv = append([]string{"timeout", "--kill-after=5s", seconds}, argv...)
			}
			result, err = s.Agent.Execute(ctx, argv, input)
		}
	}
	if err != nil {
		result.Error, result.Code = err.Error(), 125
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) servePut(w http.ResponseWriter, req *http.Request) {
	if req.ContentLength > transferlimits.Upload {
		http.Error(w, "upload exceeds maximum payload size", http.StatusRequestEntityTooLarge)
		return
	}
	path := req.Header.Get("X-Path")
	mode, err := strconv.ParseUint(req.Header.Get("X-Mode"), 8, 32)
	if path == "" || err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	duration, err := time.ParseDuration(req.Header.Get("X-Timeout"))
	if err != nil || duration <= 0 {
		http.Error(w, "invalid timeout", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), duration)
	defer cancel()
	guestOS, err := s.Agent.OS(ctx)
	if err == nil {
		parentCommand := []string{"mkdir", "-p", filepath.Dir(path)}
		if isWindowsOS(guestOS) {
			parent := "."
			if separator := strings.LastIndexAny(path, `\\/`); separator >= 0 {
				parent = path[:separator]
			}
			quoted := strings.ReplaceAll(parent, "'", "''")
			parentCommand = []string{`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, "-NoProfile", "-NonInteractive", "-Command", "[System.IO.Directory]::CreateDirectory('" + quoted + "') | Out-Null"}
		}
		var prepared Result
		prepared, err = s.Agent.Execute(ctx, parentCommand, nil)
		if err == nil && prepared.Code != 0 {
			err = fmt.Errorf("create parent directory: %s", prepared.Stderr)
		}
	}
	if err == nil {
		err = s.Agent.Upload(ctx, http.MaxBytesReader(w, req.Body, transferlimits.Upload), path)
	}
	if err == nil && !isWindowsOS(guestOS) {
		var changed Result
		changed, err = s.Agent.Execute(ctx, []string{"chmod", strconv.FormatUint(mode, 8), path}, nil)
		if err == nil && changed.Code != 0 {
			err = fmt.Errorf("chmod: %s", changed.Stderr)
		}
	}
	result := Result{}
	if err != nil {
		result.Error, result.Code = err.Error(), 125
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func Serve(execSocket string, agent *Agent) error {
	if execSocket == "" {
		execSocket = ExecSocket
	}
	if err := os.Remove(execSocket); err != nil && !os.IsNotExist(err) {
		return err
	}
	listener, err := net.Listen("unix", execSocket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(execSocket, 0o600); err != nil {
		return err
	}
	return http.Serve(listener, &Server{Agent: agent})
}

func waitForSocket(ctx context.Context, path string) error {
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func Run(ctx context.Context, execSocket string, timeout time.Duration, stdin io.Reader, stdout, stderr io.Writer, argv []string) int {
	if execSocket == "" {
		execSocket = ExecSocket
	}
	if err := waitForSocket(ctx, execSocket); err != nil {
		fmt.Fprintln(stderr, err)
		return 125
	}
	request, err := http.NewRequestWithContext(ctx, "POST", "http://guest/exec", stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 125
	}
	encoded, _ := json.Marshal(argv)
	request.Header.Set("X-Argv", base64.StdEncoding.EncodeToString(encoded))
	request.Header.Set("X-Timeout", timeout.String())
	if stdin != nil {
		request.Header.Set("X-Stdin", "1")
	}
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", execSocket)
	}}}
	reply, err := client.Do(request)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 125
	}
	defer reply.Body.Close()
	var result Result
	if err := json.NewDecoder(reply.Body).Decode(&result); err != nil {
		fmt.Fprintln(stderr, err)
		return 125
	}
	_, _ = stdout.Write(result.Stdout)
	_, _ = stderr.Write(result.Stderr)
	if result.Error != "" {
		fmt.Fprintln(stderr, strings.TrimSpace(result.Error))
	}
	return result.Code
}

func Put(ctx context.Context, execSocket string, timeout time.Duration, input io.Reader, path string, mode uint32, stderr io.Writer) int {
	if execSocket == "" {
		execSocket = ExecSocket
	}
	if err := waitForSocket(ctx, execSocket); err != nil {
		fmt.Fprintln(stderr, err)
		return 125
	}
	request, err := http.NewRequestWithContext(ctx, "POST", "http://guest/put", input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 125
	}
	request.Header.Set("X-Path", path)
	request.Header.Set("X-Mode", strconv.FormatUint(uint64(mode), 8))
	request.Header.Set("X-Timeout", timeout.String())
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", execSocket)
	}}}
	reply, err := client.Do(request)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 125
	}
	defer reply.Body.Close()
	var result Result
	if err := json.NewDecoder(reply.Body).Decode(&result); err != nil {
		fmt.Fprintln(stderr, err)
		return 125
	}
	if result.Error != "" {
		fmt.Fprintln(stderr, strings.TrimSpace(result.Error))
	}
	return result.Code
}

func Main() {
	if len(os.Args) < 2 {
		log.Fatal("labcontainers-guest serve | exec <timeout> <stdin:0|1> <argv...> | put <timeout> <mode> <path>")
	}
	if os.Args[1] == "serve" {
		log.Fatal(Serve(ExecSocket, &Agent{}))
		return
	}
	if os.Args[1] == "put" {
		if len(os.Args) != 5 {
			log.Fatal("invalid put command")
		}
		duration, err := time.ParseDuration(os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		mode, err := strconv.ParseUint(os.Args[3], 8, 32)
		if err != nil {
			log.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), duration+10*time.Second)
		defer cancel()
		os.Exit(Put(ctx, ExecSocket, duration, os.Stdin, os.Args[4], uint32(mode), os.Stderr))
	}
	if len(os.Args) < 5 || os.Args[1] != "exec" {
		log.Fatal("invalid command")
	}
	duration, err := time.ParseDuration(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration+10*time.Second)
	defer cancel()
	var input io.Reader
	if os.Args[3] == "1" {
		input = os.Stdin
	}
	os.Exit(Run(ctx, ExecSocket, duration, input, os.Stdout, os.Stderr, os.Args[4:]))
}
