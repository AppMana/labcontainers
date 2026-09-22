package server

import (
	"context"
	"os"
	"path/filepath"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/internal/session"
	"github.com/appmana/labcontainers/pkg/spec"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PlanTopology delegates to native Containerlab dry-run. Proposed topologies
// remain drafts: neither the authoritative topology nor runtime is changed.
func (s *Server) PlanTopology(ctx context.Context, req *labv1.PlanTopologyRequest) (*labv1.NativeApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.record(req.GetSessionId())
	if err != nil {
		return nil, err
	}
	path := r.TopologyPath
	if req.GetTopology() != nil {
		prepared, err := prepareDraft(r, req.GetTopology())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		// Keep the native lab directory context: Containerlab locates its
		// reconciliation baseline relative to the topology's parent directory.
		// A separate parent would silently lose configuration-drift detection.
		draft, err := os.CreateTemp(filepath.Dir(r.TopologyPath), ".plan-*.clab.yml")
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		path = draft.Name()
		defer os.Remove(path)
		if _, err := draft.Write(prepared.YAML); err != nil {
			draft.Close()
			return nil, status.Error(codes.Internal, err.Error())
		}
		if err := draft.Close(); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	result, err := s.Backend.Plan(ctx, path)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "Containerlab plan: %v", err)
	}
	return &labv1.NativeApplyResult{Json: result}, nil
}

func prepareDraft(r *session.Record, source *labv1.TopologySource) (*spec.Prepared, error) {
	raw, base, err := topologyBytes(source)
	if err != nil {
		return nil, err
	}
	prepared, err := spec.PrepareWithOptions(raw, r.Name, r.ID, r.AllowExternalAccess, spec.PrepareOptions{BaseDir: base})
	if err != nil {
		return nil, err
	}
	// Existing attachments survive on retained nodes; removed nodes do not
	// leak extension binds into the proposed topology.
	binds := map[string][]string{}
	for _, name := range prepared.Nodes {
		n := r.Nodes[name]
		if n == nil {
			continue
		}
		for _, disk := range n.Disks {
			binds[name] = append(binds[name], disk.Path+":/labcontainers-disks/"+disk.Name+".raw")
		}
		if n.BootstrapPath != "" {
			target, err := bootstrapTarget(n.BootstrapFormat)
			if err != nil {
				return nil, err
			}
			binds[name] = append(binds[name], n.BootstrapPath+":"+target+":ro")
		}
	}
	return spec.PrepareWithOptions(raw, r.Name, r.ID, r.AllowExternalAccess, spec.PrepareOptions{BaseDir: base, NodeBinds: binds})
}
