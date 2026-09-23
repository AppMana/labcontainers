package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
)

var ErrNoPersistentSession = errors.New("no persistent Labcontainers session")

// PersistentOptions describes a lab whose creator and destroyer may be
// different processes, as is common for command-line integration harnesses.
type PersistentOptions struct {
	StateDir string
	RefPath  string
	LabdPath string
	TTL      time.Duration
}

type persistentRef struct {
	ID     string `json:"id"`
	Socket string `json:"socket,omitempty"`
}

// DeployPersistent starts and keeps a session, then writes the opaque session
// reference needed by a later DestroyPersistent call.
func DeployPersistent(ctx context.Context, opts PersistentOptions, spec *labv1.LabSpec) (*labv1.Session, error) {
	if opts.StateDir == "" || opts.RefPath == "" {
		return nil, fmt.Errorf("state directory and reference path are required")
	}
	if _, err := os.Lstat(opts.RefPath); err == nil {
		return nil, fmt.Errorf("persistent reference already exists: %s; reconnect or destroy it explicitly", opts.RefPath)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if opts.TTL <= 0 {
		opts.TTL = 7 * 24 * time.Hour
	}
	c, err := Launch(ctx, Options{StateDir: opts.StateDir, LabdPath: opts.LabdPath})
	if err != nil {
		return nil, err
	}
	defer c.Close()
	session, err := c.Start(ctx, spec, opts.TTL)
	if err != nil {
		return nil, err
	}
	if err := session.Keep(ctx, opts.TTL); err != nil {
		_ = session.Destroy(context.Background())
		return nil, err
	}
	if err := writePersistentRef(opts.RefPath, persistentRef{ID: session.ID(), Socket: c.Socket()}); err != nil {
		_ = session.Destroy(context.Background())
		return nil, err
	}
	return session.value, nil
}

// OpenPersistent reconnects to the existing daemon without launching another
// owner of its state directory. The caller must close the returned client.
// Missing/unreachable sockets require explicit daemon recovery; they do not
// authorize a competing daemon or adoption of a same-named topology.
func OpenPersistent(ctx context.Context, opts PersistentOptions) (*Client, *Session, error) {
	ref, err := readPersistentRef(opts.RefPath)
	if err != nil {
		return nil, nil, err
	}
	if ref.Socket == "" {
		return nil, nil, fmt.Errorf("persistent reference %s has no daemon socket; explicit legacy session recovery is required", opts.RefPath)
	}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c, err := Dial(dialCtx, ref.Socket)
	if err != nil {
		return nil, nil, fmt.Errorf("reconnect persistent daemon: %w", err)
	}
	session, err := c.Resume(ctx, ref.ID)
	if err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	return c, session, nil
}

// DestroyPersistent reconnects to a kept session and destroys its topology.
func DestroyPersistent(ctx context.Context, opts PersistentOptions) error {
	c, session, err := OpenPersistent(ctx, opts)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := session.Destroy(ctx); err != nil {
		return err
	}
	return os.Remove(opts.RefPath)
}

func writePersistentRef(path string, ref persistentRef) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".labcontainers-ref-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := json.NewEncoder(tmp).Encode(ref); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func readPersistentRef(path string) (persistentRef, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return persistentRef{}, ErrNoPersistentSession
	}
	if err != nil {
		return persistentRef{}, err
	}
	var ref persistentRef
	if err := json.Unmarshal(b, &ref); err != nil || ref.ID == "" {
		return persistentRef{}, fmt.Errorf("invalid persistent session reference %s", path)
	}
	return ref, nil
}
