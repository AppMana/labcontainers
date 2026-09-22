package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/internal/engine"
	serverpkg "github.com/appmana/labcontainers/internal/server"
	"github.com/appmana/labcontainers/internal/session"
	"google.golang.org/grpc"
)

func main() {
	var socket, stateDir string
	var parentPID int
	flag.StringVar(&socket, "socket", "", "private Unix socket path")
	flag.StringVar(&stateDir, "state-dir", "", "persistent session state directory")
	flag.IntVar(&parentPID, "parent-pid", 0, "owning SDK process")
	flag.Parse()
	if socket == "" {
		fmt.Fprintln(os.Stderr, "labd: --socket is required")
		os.Exit(2)
	}
	if err := run(socket, stateDir, parentPID); err != nil {
		fmt.Fprintln(os.Stderr, "labd:", err)
		os.Exit(1)
	}
}

func run(socket, stateDir string, parentPID int) error {
	store, err := session.NewStore(stateDir)
	if err != nil {
		return err
	}
	backend := engine.NewContainerlab()
	service := serverpkg.New(store, backend)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	_ = service.Scavenge(ctx, time.Now().UTC())
	cancel()

	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("refusing to replace non-socket %s", socket)
		}
		if err := os.Remove(socket); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	lis, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer lis.Close()
	defer os.Remove(socket)
	if err := os.Chmod(socket, 0o600); err != nil {
		return err
	}

	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(labv1.MaxMessageBytes), grpc.MaxSendMsgSize(labv1.MaxMessageBytes))
	labv1.RegisterLabcontainersServer(grpcServer, service)
	done := make(chan error, 1)
	go func() { done <- grpcServer.Serve(lis) }()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	parentGone := make(chan struct{}, 1)
	if parentPID > 0 {
		go watchParent(parentPID, parentGone)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	detached := false
	for {
		ownerCleanup := false
		select {
		case err := <-done:
			return err
		case <-stop:
			if detached {
				// An explicit second signal stops an already detached daemon.
				// Session records remain recoverable by another daemon.
				grpcServer.GracefulStop()
				return nil
			}
			detached = true
			ownerCleanup = true
			parentGone = nil
		case <-parentGone:
			detached = true
			ownerCleanup = true
			parentGone = nil
		case <-ticker.C:
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		if ownerCleanup {
			if err := service.CleanupUnkept(cleanupCtx); err != nil {
				fmt.Fprintln(os.Stderr, "labd: owner cleanup:", err)
			}
		}
		if err := service.Scavenge(cleanupCtx, time.Now().UTC()); err != nil {
			fmt.Fprintln(os.Stderr, "labd: lease cleanup:", err)
		}
		cleanupCancel()
		if detached {
			records, err := store.List()
			if err != nil {
				fmt.Fprintln(os.Stderr, "labd: retained state:", err)
				continue
			}
			if len(records) == 0 {
				grpcServer.GracefulStop()
				return nil
			}
		}
	}
}

func watchParent(pid int, gone chan<- struct{}) {
	path := filepath.Join("/proc", strconv.Itoa(pid))
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			gone <- struct{}{}
			return
		}
	}
}
