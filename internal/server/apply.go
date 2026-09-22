package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/internal/session"
	"github.com/srl-labs/containerlab/core"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ApplyTopology rechecks caller-approved native impact before convergence.
// Native apply is not transactional: failures retain the desired topology and
// diagnostics for inspection/retry, rather than claiming an automatic rollback.
func (s *Server) ApplyTopology(ctx context.Context, req *labv1.ApplyTopologyRequest) (_ *labv1.Session, returnErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var approved core.ApplyResult
	if json.Unmarshal(req.GetApprovedPlan().GetJson(), &approved) != nil || !approved.DryRun {
		return nil, status.Error(codes.InvalidArgument, "an approved native dry-run plan is required")
	}
	r, err := s.record(req.GetSessionId())
	if err != nil {
		return nil, err
	}
	for _, fault := range r.Faults {
		if fault.Active {
			return nil, status.Error(codes.FailedPrecondition, "revert active faults before reconciling topology")
		}
	}
	if err := s.Backend.CheckSessionOwnership(ctx, r.Name, r.ID); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	prepared, err := prepareDraft(r, req.GetTopology())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	nodes := map[string]*session.Node{}
	for _, name := range prepared.Nodes {
		if old := r.Nodes[name]; old != nil {
			copy := *old
			nodes[name] = &copy
		} else {
			nodes[name] = &session.Node{Name: name, Control: "container", State: "created"}
		}
	}
	for name, ext := range req.GetNodes() {
		n := nodes[name]
		if n == nil {
			return nil, status.Errorf(codes.InvalidArgument, "unknown node extension %q", name)
		}
		if len(ext.GetDisks()) != 0 || ext.GetBootstrap() != nil {
			return nil, status.Error(codes.Unimplemented, "dynamic disk/bootstrap changes are not implemented; existing attachments persist")
		}
		if ext.GetControl() != "" && ext.GetControl() != "container" && ext.GetControl() != "qga" {
			return nil, status.Error(codes.InvalidArgument, "control must be container or qga")
		}
		if ext.GetControl() != "" {
			n.Control = ext.GetControl()
		}
	}
	before, err := os.ReadFile(r.TopologyPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	// Keep native labdir/state discovery at the canonical path. Before any
	// runtime mutation, every error restores the original desired topology.
	if err := writeTopology(r.TopologyPath, prepared.YAML); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	mutating := false
	defer func() {
		if !mutating {
			returnErr = errors.Join(returnErr, writeTopology(r.TopologyPath, before))
		}
	}()
	if err := s.Backend.Validate(ctx, r.TopologyPath); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	rawPlan, err := s.Backend.Plan(ctx, r.TopologyPath)
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	var current core.ApplyResult
	if err := json.Unmarshal(rawPlan, &current); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if !reflect.DeepEqual(approved, current) {
		s.event(r, "topology.plan.changed", json.RawMessage(rawPlan))
		return nil, status.Errorf(codes.FailedPrecondition, "native plan changed; review a fresh plan before applying: %s", rawPlan)
	}
	operation, err := randomID(8)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	for suffix, value := range map[string][]byte{"before": before, "desired": prepared.YAML, "plan": rawPlan} {
		if err := os.WriteFile(filepath.Join(r.ArtifactDirectory, "apply-"+operation+"-"+suffix), value, 0o600); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	// Union node metadata until success: partial native failures may leave old
	// or new nodes alive. Destruction still uses exact session topology labels.
	for name, node := range nodes {
		r.Nodes[name] = node
	}
	r.State = "reconciling"
	if err := s.Store.Save(r); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	mutating = true
	s.event(r, "topology.apply.requested", json.RawMessage(rawPlan))
	err = s.Backend.Deploy(ctx, r.TopologyPath)
	if err == nil {
		for _, name := range prepared.Nodes {
			if err = s.Backend.RestoreAttachments(ctx, r.TopologyPath, name); err != nil {
				break
			}
		}
	}
	if err == nil {
		err = s.Backend.ProofIsolation(ctx, r.Name, prepared.IsolatedNodes)
	}
	if err != nil {
		r.State = "reconcile-failed"
		_ = s.Store.Save(r)
		s.event(r, "topology.apply.failed", map[string]any{"error": err.Error()})
		return nil, status.Errorf(codes.Internal, "native apply failed; partial state retained for retry or destroy (evidence: %s): %v", r.ArtifactDirectory, err)
	}
	r.Nodes, r.State = nodes, "running"
	for _, node := range r.Nodes {
		node.State = "running"
	}
	if err := s.Store.Save(r); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.event(r, "topology.applied", map[string]any{"operation": operation})
	return protoSession(r), nil
}

func writeTopology(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".topology-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
