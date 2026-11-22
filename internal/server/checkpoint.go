package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/internal/session"
	"github.com/appmana/labcontainers/pkg/checkpoint"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ExtendSession moves the lease of a session that is still in use. It does
// not mark the session kept: the owner's exit still destroys it.
func (s *Server) ExtendSession(_ context.Context, req *labv1.ExtendSessionRequest) (*labv1.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.record(req.GetId())
	if err != nil {
		return nil, err
	}
	ttl := time.Duration(req.GetTtlSeconds()) * time.Second
	if ttl <= 0 {
		return nil, status.Error(codes.InvalidArgument, "a positive ttl is required")
	}
	// Only ever move the lease later: a short renewal must not cut a longer
	// lease the session already holds.
	if expires := time.Now().UTC().Add(ttl); expires.After(r.Expires) {
		r.Expires = expires
		if err := s.Store.Save(r); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return protoSession(r), nil
}

func checkpointDirectory(value string) (string, error) {
	if !filepath.IsAbs(value) || filepath.Clean(value) == string(filepath.Separator) {
		return "", status.Error(codes.InvalidArgument, "checkpoint directory must be an absolute non-root path")
	}
	return filepath.Clean(value), nil
}

// checkpointNodes selects the requested nodes, or every node that can own a
// disk, and requires each to be stopped: a running guest's disks are neither
// consistent to read nor safe to replace.
func checkpointNodes(r *session.Record, requested []string) ([]*session.Node, error) {
	var nodes []*session.Node
	if len(requested) == 0 {
		for _, n := range r.Nodes {
			if n.Control == "qga" {
				nodes = append(nodes, n)
			}
		}
	}
	seen := map[string]bool{}
	for _, name := range requested {
		n := r.Nodes[name]
		if n == nil || seen[name] {
			return nil, status.Errorf(codes.InvalidArgument, "unknown or repeated checkpoint node %q", name)
		}
		seen[name] = true
		nodes = append(nodes, n)
	}
	if len(nodes) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "the session has no node with disks")
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	for _, n := range nodes {
		if n.State != "stopped" {
			return nil, status.Errorf(codes.FailedPrecondition, "node %q is %s; stop it before copying its disks", n.Name, n.State)
		}
	}
	return nodes, nil
}

func (s *Server) CreateCheckpoint(ctx context.Context, req *labv1.CheckpointRequest) (*labv1.Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.record(req.GetSessionId())
	if err != nil {
		return nil, err
	}
	dir, err := checkpointDirectory(req.GetDirectory())
	if err != nil {
		return nil, err
	}
	nodes, err := checkpointNodes(r, req.GetNodes())
	if err != nil {
		return nil, err
	}
	// Never overwrite: a checkpoint is the reference later runs start from.
	if entries, err := os.ReadDir(dir); err == nil && len(entries) != 0 {
		return nil, status.Errorf(codes.AlreadyExists, "checkpoint directory %s is not empty", dir)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	manifest := checkpoint.Manifest{Created: time.Now().UTC(), Session: r.ID, Labels: req.GetLabels(), Nodes: map[string]checkpoint.Node{}}
	for _, n := range nodes {
		image, files, err := s.Backend.ExportNode(ctx, r.TopologyPath, n.Name, filepath.Dir(checkpoint.RootPath(dir, n.Name, "x")))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "checkpoint node %s: %v", n.Name, err)
		}
		saved := checkpoint.Node{Image: image, Root: files}
		for _, disk := range n.Disks {
			if err := s.Backend.CopyDisk(ctx, disk.Path, checkpoint.DiskPath(dir, n.Name, disk.Name)); err != nil {
				return nil, status.Errorf(codes.Internal, "checkpoint disk %s/%s: %v", n.Name, disk.Name, err)
			}
			saved.Disks = append(saved.Disks, checkpoint.Disk{Name: disk.Name, SizeBytes: disk.SizeBytes})
		}
		if len(saved.Root) == 0 && len(saved.Disks) == 0 {
			return nil, status.Errorf(codes.FailedPrecondition, "node %s has no disk content to save", n.Name)
		}
		manifest.Nodes[n.Name] = saved
	}
	if err := checkpoint.Write(dir, manifest); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.event(r, "session.checkpoint.created", map[string]any{"directory": dir, "nodes": manifest.NodeNames(), "labels": manifest.Labels})
	return protoCheckpoint(dir, manifest), nil
}

func (s *Server) RestoreCheckpoint(ctx context.Context, req *labv1.CheckpointRequest) (*labv1.Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.record(req.GetSessionId())
	if err != nil {
		return nil, err
	}
	dir, err := checkpointDirectory(req.GetDirectory())
	if err != nil {
		return nil, err
	}
	manifest, err := checkpoint.Read(dir)
	if errors.Is(err, checkpoint.ErrNone) {
		return nil, status.Errorf(codes.NotFound, "%s holds no complete checkpoint", dir)
	}
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	requested := req.GetNodes()
	if len(requested) == 0 {
		requested = manifest.NodeNames()
	}
	nodes, err := checkpointNodes(r, requested)
	if err != nil {
		return nil, err
	}
	// Check every node before changing any, so a mismatch leaves all disks
	// as they were.
	for _, n := range nodes {
		saved, ok := manifest.Nodes[n.Name]
		if !ok {
			return nil, status.Errorf(codes.FailedPrecondition, "the checkpoint holds no node %q", n.Name)
		}
		if len(saved.Disks) != len(n.Disks) {
			return nil, status.Errorf(codes.FailedPrecondition, "node %q declares %d disks; the checkpoint holds %d", n.Name, len(n.Disks), len(saved.Disks))
		}
		for i, disk := range n.Disks {
			if saved.Disks[i].Name != disk.Name || saved.Disks[i].SizeBytes != disk.SizeBytes {
				return nil, status.Errorf(codes.FailedPrecondition, "node %q disk %q does not match the checkpoint", n.Name, disk.Name)
			}
		}
	}
	s.event(r, "session.checkpoint.restoring", map[string]any{"directory": dir, "nodes": requested})
	for _, n := range nodes {
		saved := manifest.Nodes[n.Name]
		if err := s.Backend.ImportNode(ctx, r.TopologyPath, n.Name, filepath.Dir(checkpoint.RootPath(dir, n.Name, "x")), saved.Image, saved.Root); err != nil {
			return nil, status.Errorf(codes.Internal, "restore node %s: %v", n.Name, err)
		}
		for _, disk := range n.Disks {
			if err := s.Backend.CopyDisk(ctx, checkpoint.DiskPath(dir, n.Name, disk.Name), disk.Path); err != nil {
				return nil, status.Errorf(codes.Internal, "restore disk %s/%s: %v", n.Name, disk.Name, err)
			}
		}
	}
	s.event(r, "session.checkpoint.restored", map[string]any{"directory": dir, "nodes": requested, "labels": manifest.Labels})
	return protoCheckpoint(dir, manifest), nil
}

func protoCheckpoint(dir string, m checkpoint.Manifest) *labv1.Checkpoint {
	out := &labv1.Checkpoint{Directory: dir, CreatedUnix: m.Created.Unix(), Labels: m.Labels}
	for _, name := range m.NodeNames() {
		node := m.Nodes[name]
		files := append([]string(nil), node.Root...)
		for _, disk := range node.Disks {
			files = append(files, disk.Name+".raw")
		}
		out.Nodes = append(out.Nodes, &labv1.CheckpointNode{Name: name, Image: node.Image, Files: files})
	}
	out.SizeBytes, _ = checkpoint.Size(dir)
	return out
}
