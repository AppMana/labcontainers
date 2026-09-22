package client

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"google.golang.org/grpc"
)

func TestPersistentReferenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "session.json")
	if err := writePersistentRef(path, persistentRef{ID: "session-id", Socket: "/tmp/control.sock"}); err != nil {
		t.Fatal(err)
	}
	got, err := readPersistentRef(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "session-id" || got.Socket != "/tmp/control.sock" {
		t.Fatalf("id = %q", got.ID)
	}
}

type persistentServer struct {
	labv1.UnimplementedLabcontainersServer
	mu        sync.Mutex
	destroyed bool
}

func (s *persistentServer) GetSession(_ context.Context, ref *labv1.SessionRef) (*labv1.Session, error) {
	return &labv1.Session{Id: ref.Id, ResumeToken: "token"}, nil
}

func (s *persistentServer) DestroySession(_ context.Context, request *labv1.DestroySessionRequest) (*labv1.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !request.PreserveKept && request.Id == "owned" && request.ResumeToken == "token" {
		s.destroyed = true
	}
	return &labv1.Empty{}, nil
}

func TestPersistentReconnectUsesOriginalDaemon(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "lab.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	service := &persistentServer{}
	labv1.RegisterLabcontainersServer(server, service)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	opts := PersistentOptions{RefPath: filepath.Join(dir, "ref.json"), LabdPath: "/nonexistent/must-not-launch"}
	if err := writePersistentRef(opts.RefPath, persistentRef{ID: "owned", Socket: socket}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, session, err := OpenPersistent(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if session.ID() != "owned" || c.Socket() != socket || c.cmd != nil {
		t.Fatal("not connected to original daemon")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	destroyed := service.destroyed
	service.mu.Unlock()
	if destroyed {
		t.Fatal("closing inspection client destroyed kept session")
	}
	if err := DestroyPersistent(ctx, opts); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	destroyed = service.destroyed
	service.mu.Unlock()
	if !destroyed {
		t.Fatal("original daemon did not receive explicit teardown")
	}
	if _, err := os.Stat(opts.RefPath); !os.IsNotExist(err) {
		t.Fatalf("reference remains: %v", err)
	}
}

func TestPersistentLegacyReferenceRequiresExplicitRecovery(t *testing.T) {
	opts := PersistentOptions{RefPath: filepath.Join(t.TempDir(), "ref.json"), LabdPath: "/nonexistent/must-not-launch"}
	if err := writePersistentRef(opts.RefPath, persistentRef{ID: "legacy"}); err != nil {
		t.Fatal(err)
	}
	err := DestroyPersistent(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "explicit legacy session recovery") {
		t.Fatalf("unexpected result: %v", err)
	}
	if _, err := os.Stat(opts.RefPath); err != nil {
		t.Fatal("lost recovery reference", err)
	}
}

func TestDeployPersistentDoesNotOverwriteReference(t *testing.T) {
	dir := t.TempDir()
	opts := PersistentOptions{StateDir: dir, RefPath: filepath.Join(dir, "ref.json"), LabdPath: "/nonexistent/must-not-launch"}
	if err := writePersistentRef(opts.RefPath, persistentRef{ID: "owned"}); err != nil {
		t.Fatal(err)
	}
	_, err := DeployPersistent(context.Background(), opts, nil)
	if err == nil || !strings.Contains(err.Error(), "reference already exists") {
		t.Fatalf("unexpected result: %v", err)
	}
	ref, err := readPersistentRef(opts.RefPath)
	if err != nil || ref.ID != "owned" {
		t.Fatalf("reference changed: %+v %v", ref, err)
	}
}

func TestMissingPersistentReferenceIsDistinct(t *testing.T) {
	_, err := readPersistentRef(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, ErrNoPersistentSession) {
		t.Fatalf("error = %v", err)
	}
}
