package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
)

// Exercise the real Launch/Close process ownership without VMs or privileges.
// A kept daemon must not retain the invoking go test command's output pipe.
func TestRetainedDaemonDoesNotHoldOwnerOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix daemon launcher and shell helper")
	}
	mode := os.Getenv("LABCONTAINERS_OUTPUT_TEST_HELPER")
	if mode == "daemon" {
		signal.Ignore(os.Interrupt)
		if err := os.WriteFile(os.Getenv("LABCONTAINERS_OUTPUT_TEST_PID"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			panic(err)
		}
		var socket string
		for i, arg := range os.Args {
			if arg == "--socket" && i+1 < len(os.Args) {
				socket = os.Args[i+1]
			}
		}
		listener, err := net.Listen("unix", socket)
		if err != nil {
			panic(err)
		}
		fmt.Fprintln(os.Stderr, "retained-daemon-diagnostic")
		if err := grpc.NewServer().Serve(listener); err != nil {
			panic(err)
		}
		return
	}
	if mode == "owner" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		root := os.Getenv("LABCONTAINERS_OUTPUT_TEST_ROOT")
		c, err := Launch(ctx, Options{Socket: filepath.Join(root, "labd.sock"), StateDir: filepath.Join(root, "state"), LabdPath: filepath.Join(root, "fake-labd")})
		if err != nil {
			t.Fatal(err)
		}
		if c.DaemonLogPath() != filepath.Join(root, "labd.sock.log") {
			t.Fatalf("unexpected daemon log path: %q", c.DaemonLogPath())
		}
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		fmt.Println("owner-closed")
		return
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "state", "sessions", "kept"), 0700); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The helper accepts labd's arguments after Go's flag delimiter.
	script := "#!/bin/sh\nexport LABCONTAINERS_OUTPUT_TEST_HELPER=daemon\nexec \"$LABCONTAINERS_OUTPUT_TEST_BINARY\" -test.run '^TestRetainedDaemonDoesNotHoldOwnerOutput$' -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(root, "fake-labd"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(root, "daemon.pid")
	t.Cleanup(func() {
		if raw, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(string(raw)); err == nil {
				if p, err := os.FindProcess(pid); err == nil {
					_ = p.Kill()
				}
			}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, "-test.run", "^TestRetainedDaemonDoesNotHoldOwnerOutput$")
	cmd.Env = append(os.Environ(), "LABCONTAINERS_OUTPUT_TEST_HELPER=owner", "LABCONTAINERS_OUTPUT_TEST_ROOT="+root, "LABCONTAINERS_OUTPUT_TEST_PID="+pidFile, "LABCONTAINERS_OUTPUT_TEST_BINARY="+self)
	cmd.WaitDelay = 250 * time.Millisecond
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("owner returned but retained daemon held its output: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "owner-closed") {
		t.Fatalf("owner did not finish: %s", out)
	}
	if strings.Contains(string(out), "retained-daemon-diagnostic") {
		t.Fatal("daemon still writes to its owner's output")
	}
	log, err := os.ReadFile(filepath.Join(root, "labd.sock.log"))
	if err != nil || !strings.Contains(string(log), "retained-daemon-diagnostic") {
		t.Fatalf("retained daemon diagnostics lost: %s (%v)", log, err)
	}
	inspector, err := Dial(ctx, filepath.Join(root, "labd.sock"))
	if err != nil {
		t.Fatalf("retained daemon did not survive owner exit: %v", err)
	}
	if err := inspector.Close(); err != nil {
		t.Fatal(err)
	}
}
