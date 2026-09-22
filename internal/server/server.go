package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/internal/engine"
	"github.com/appmana/labcontainers/internal/session"
	"github.com/appmana/labcontainers/pkg/bootstrap"
	"github.com/appmana/labcontainers/pkg/spec"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const defaultTTL = 2 * time.Hour

var diskNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
var labNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`)

type Backend interface {
	Doctor(context.Context) error
	Validate(context.Context, string) error
	Deploy(context.Context, string) error
	ProofIsolation(context.Context, string, []string) error
	ContainerName(context.Context, string, string) (string, error)
	Destroy(context.Context, string) error
	Lifecycle(context.Context, string, string, string) error
	Replace(context.Context, string, string) error
	Exec(context.Context, string, string, string, time.Duration, []byte, []string) (engine.Result, error)
	Put(context.Context, string, string, string, string, uint32, []byte) error
	SetLink(context.Context, string, string, string, string, bool) error
	Netem(context.Context, string, string, time.Duration, time.Duration, float64, uint64, float64) error
	ResetNetem(context.Context, string, string) error
}

type Server struct {
	labv1.UnimplementedLabcontainersServer
	Store   *session.Store
	Backend Backend
	mu      sync.Mutex
}

func New(store *session.Store, backend Backend) *Server {
	return &Server{Store: store, Backend: backend}
}

func (s *Server) CreateSession(ctx context.Context, req *labv1.CreateSessionRequest) (*labv1.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.GetSpec() == nil || req.GetSpec().GetTopology() == nil {
		return nil, status.Error(codes.InvalidArgument, "spec.topology is required")
	}
	if err := s.Backend.Doctor(ctx); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "host prerequisites: %v", err)
	}
	id, err := randomID(16)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	name := req.GetSpec().GetName()
	if name == "" {
		name = "lc-" + id[:12]
	} else if !labNamePattern.MatchString(name) {
		return nil, status.Error(codes.InvalidArgument, "spec.name must be 1-63 letters, digits, underscores, or hyphens")
	}
	if records, listErr := s.Store.List(); listErr != nil {
		return nil, status.Error(codes.Internal, listErr.Error())
	} else {
		for _, existing := range records {
			if existing.Name == name {
				return nil, status.Errorf(codes.AlreadyExists, "lab name %q belongs to session %s", name, existing.ID)
			}
		}
	}
	topoBytes, topologyDir, err := topologyBytes(req.GetSpec().GetTopology())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	dir := s.Store.SessionDir(id)
	binds, disks, err := diskPlan(dir, req.GetSpec().GetNodes())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	bootstraps, bootstrapBinds, err := bootstrapPlan(dir, req.GetSpec().GetNodes())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	for nodeName, values := range bootstrapBinds {
		binds[nodeName] = append(binds[nodeName], values...)
	}
	prepared, err := spec.PrepareWithOptions(topoBytes, name, id, req.GetSpec().GetAllowExternalAccess(), spec.PrepareOptions{NodeBinds: binds, BaseDir: topologyDir})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	knownNodes := map[string]bool{}
	for _, nodeName := range prepared.Nodes {
		knownNodes[nodeName] = true
	}
	for nodeName := range req.GetSpec().GetNodes() {
		if !knownNodes[nodeName] {
			return nil, status.Errorf(codes.InvalidArgument, "node extension refers to unknown topology node %q", nodeName)
		}
	}

	ttl := time.Duration(req.GetTtlSeconds()) * time.Second
	if ttl <= 0 {
		ttl = defaultTTL
	}
	resume, err := randomID(32)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	artifacts := filepath.Join(dir, "artifacts")
	if requested := req.GetSpec().GetArtifactDirectory(); requested != "" {
		artifacts = requested
	}
	record := &session.Record{
		ID: id, Name: prepared.Name, State: "provisioning",
		TopologyPath: filepath.Join(dir, "topology.clab.yml"), ArtifactDirectory: artifacts,
		Expires: time.Now().UTC().Add(ttl), ResumeToken: resume,
		Nodes: map[string]*session.Node{}, Faults: map[string]*session.Fault{}, Labels: req.GetLabels(),
	}
	for _, nodeName := range prepared.Nodes {
		control := "container"
		if ext := req.GetSpec().GetNodes()[nodeName]; ext != nil && ext.GetControl() != "" {
			control = ext.GetControl()
		}
		n := &session.Node{Name: nodeName, Control: control, State: "created", Disks: disks[nodeName]}
		if b, ok := bootstraps[nodeName]; ok {
			n.BootstrapFormat, n.BootstrapPath = b.Format, b.Path
		}
		record.Nodes[nodeName] = n
	}
	if err := s.Store.Create(record); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := createDisks(disks); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := writeBootstraps(bootstraps); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := os.MkdirAll(artifacts, 0o700); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := os.WriteFile(record.TopologyPath, prepared.YAML, 0o600); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.event(record, "session.prepared", map[string]any{"nodes": prepared.Nodes})
	if err := s.Backend.Validate(ctx, record.TopologyPath); err != nil {
		record.State = "failed"
		_ = s.Store.Save(record)
		return nil, status.Errorf(codes.InvalidArgument, "Containerlab validation failed for session %s: %v", id, err)
	}
	if err := s.Backend.Deploy(ctx, record.TopologyPath); err != nil {
		record.State = "failed"
		_ = s.Store.Save(record)
		_ = s.Backend.Destroy(context.Background(), record.TopologyPath)
		return nil, status.Errorf(codes.Internal, "deploy session %s: %v", id, err)
	}
	if err := s.Backend.ProofIsolation(ctx, record.Name, prepared.IsolatedNodes); err != nil {
		record.State = "failed"
		_ = s.Store.Save(record)
		_ = s.Backend.Destroy(context.Background(), record.TopologyPath)
		return nil, status.Errorf(codes.FailedPrecondition, "isolation proof failed for session %s: %v", id, err)
	}
	record.State = "running"
	for _, node := range record.Nodes {
		node.State = "running"
	}
	if err := s.Store.Save(record); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.event(record, "session.running", nil)
	return protoSession(record), nil
}

func (s *Server) GetSession(_ context.Context, ref *labv1.SessionRef) (*labv1.Session, error) {
	r, err := s.record(ref.GetId())
	if err != nil {
		return nil, err
	}
	return protoSession(r), nil
}

func (s *Server) DestroySession(ctx context.Context, req *labv1.DestroySessionRequest) (*labv1.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.record(req.GetId())
	if err != nil {
		return nil, err
	}
	if req.GetResumeToken() != "" && req.GetResumeToken() != r.ResumeToken {
		return nil, status.Error(codes.PermissionDenied, "invalid resume token")
	}
	if err := s.Backend.Destroy(ctx, r.TopologyPath); err != nil {
		return nil, status.Errorf(codes.Internal, "destroy session: %v", err)
	}
	s.event(r, "session.destroyed", nil)
	if err := s.Store.Delete(r.ID); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &labv1.Empty{}, nil
}

func (s *Server) KeepSession(_ context.Context, req *labv1.KeepSessionRequest) (*labv1.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.record(req.GetId())
	if err != nil {
		return nil, err
	}
	ttl := time.Duration(req.GetTtlSeconds()) * time.Second
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	r.Kept = true
	r.Expires = time.Now().UTC().Add(ttl)
	if err := s.Store.Save(r); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.event(r, "session.kept", map[string]any{"expires": r.Expires})
	return protoSession(r), nil
}

func (s *Server) Exec(ctx context.Context, req *labv1.ExecRequest) (*labv1.ExecResponse, error) {
	r, n, err := s.node(req.GetNode())
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(req.GetTimeoutMillis()) * time.Millisecond
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	result, runErr := s.Backend.Exec(ctx, r.Name, n.Name, n.Control, timeout, req.GetStdin(), req.GetArgv())
	if runErr != nil {
		return nil, status.Errorf(codes.Internal, "exec: %v", runErr)
	}
	s.event(r, "node.exec", map[string]any{"node": n.Name, "argv": req.GetArgv(), "exitCode": result.ExitCode})
	return &labv1.ExecResponse{Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: int32(result.ExitCode)}, nil
}

func (s *Server) Put(ctx context.Context, req *labv1.PutRequest) (*labv1.Empty, error) {
	r, n, err := s.node(req.GetNode())
	if err != nil {
		return nil, err
	}
	mode := req.GetMode()
	if mode == 0 {
		mode = 0o600
	}
	if err := s.Backend.Put(ctx, r.Name, n.Name, n.Control, req.GetPath(), mode, req.GetContent()); err != nil {
		return nil, status.Errorf(codes.Internal, "put: %v", err)
	}
	s.event(r, "node.put", map[string]any{"node": n.Name, "path": req.GetPath(), "mode": mode})
	return &labv1.Empty{}, nil
}

func (s *Server) Lifecycle(ctx context.Context, req *labv1.LifecycleRequest) (*labv1.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, n, err := s.node(req.GetNode())
	if err != nil {
		return nil, err
	}
	var action string
	switch req.GetAction() {
	case labv1.LifecycleAction_CRASH:
		action = "crash"
	case labv1.LifecycleAction_POWER_OFF:
		action = "stop"
	case labv1.LifecycleAction_START:
		action = "start"
	case labv1.LifecycleAction_RESTART:
		action = "restart"
	case labv1.LifecycleAction_REPLACE:
		action = "replace"
	default:
		return nil, status.Error(codes.InvalidArgument, "lifecycle action is required")
	}
	if action == "replace" {
		if len(req.GetBootstrap().GetValue()) != 0 {
			if err := s.configureBootstrap(r, n, req.GetBootstrap()); err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
			if err := s.Backend.Validate(ctx, r.TopologyPath); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "updated topology validation: %v", err)
			}
		}
		if err := resetDisks(n.Disks); err != nil {
			return nil, status.Errorf(codes.Internal, "reset node disks: %v", err)
		}
		err = s.Backend.Replace(ctx, r.TopologyPath, n.Name)
	} else {
		s.event(r, "node."+action+".requested", map[string]any{"node": n.Name})
		err = s.Backend.Lifecycle(ctx, r.TopologyPath, n.Name, action)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%s node: %v", action, err)
	}
	if action == "stop" || action == "crash" {
		n.State = "stopped"
	} else {
		n.State = "running"
	}
	if err := s.Store.Save(r); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.event(r, "node."+action, map[string]any{"node": n.Name})
	return &labv1.Node{Name: n.Name, State: n.State}, nil
}

func (s *Server) ApplyFault(ctx context.Context, req *labv1.ApplyFaultRequest) (*labv1.Fault, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.record(req.GetSessionId())
	if err != nil {
		return nil, err
	}
	id, err := randomID(12)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	f := &session.Fault{ID: id, Node: req.GetNode(), Interface: req.GetInterface(), Active: true}
	switch fault := req.GetFault().(type) {
	case *labv1.ApplyFaultRequest_Netem:
		if f.Node == "" || f.Interface == "" {
			return nil, status.Error(codes.InvalidArgument, "netem requires node and interface")
		}
		f.Kind = "netem"
		if r.Nodes[f.Node] == nil {
			return nil, status.Error(codes.InvalidArgument, "netem requires a known node")
		}
		container, err := s.Backend.ContainerName(ctx, r.Name, f.Node)
		if err != nil {
			return nil, status.Errorf(codes.FailedPrecondition, "resolve netem node: %v", err)
		}
		if err := s.Backend.Netem(ctx, container, f.Interface, time.Duration(fault.Netem.GetDelayMillis())*time.Millisecond, time.Duration(fault.Netem.GetJitterMillis())*time.Millisecond, fault.Netem.GetLossPercent(), fault.Netem.GetRateKbit(), fault.Netem.GetCorruptionPercent()); err != nil {
			return nil, status.Errorf(codes.Internal, "apply netem: %v", err)
		}
	case *labv1.ApplyFaultRequest_LinkState:
		n := r.Nodes[fault.LinkState.GetNode()]
		if n == nil || fault.LinkState.GetInterface() == "" {
			return nil, status.Error(codes.InvalidArgument, "link state requires a known node and interface")
		}
		f.Kind, f.Node, f.Interface, f.RestoreUp = "link-state", n.Name, fault.LinkState.GetInterface(), !fault.LinkState.GetUp()
		if err := s.Backend.SetLink(ctx, r.Name, n.Name, n.Control, f.Interface, fault.LinkState.GetUp()); err != nil {
			return nil, status.Errorf(codes.Internal, "set link state: %v", err)
		}
	case *labv1.ApplyFaultRequest_Partition:
		return nil, status.Error(codes.Unimplemented, "group partitions require the host bridge filter backend")
	default:
		return nil, status.Error(codes.InvalidArgument, "fault is required")
	}
	r.Faults[id] = f
	if err := s.Store.Save(r); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.event(r, "fault.applied", f)
	return &labv1.Fault{Id: id, Kind: f.Kind, Active: true}, nil
}

func (s *Server) RevertFault(ctx context.Context, ref *labv1.FaultRef) (*labv1.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.revertFault(ctx, ref.GetSessionId(), ref.GetId()); err != nil {
		return nil, err
	}
	return &labv1.Empty{}, nil
}

func (s *Server) revertFault(ctx context.Context, sessionID, faultID string) error {
	r, err := s.record(sessionID)
	if err != nil {
		return err
	}
	f := r.Faults[faultID]
	if f == nil {
		return status.Error(codes.NotFound, "fault not found")
	}
	if !f.Active {
		return nil
	}
	switch f.Kind {
	case "netem":
		container, err := s.Backend.ContainerName(ctx, r.Name, f.Node)
		if err != nil {
			return status.Errorf(codes.FailedPrecondition, "resolve netem node: %v", err)
		}
		if err := s.Backend.ResetNetem(ctx, container, f.Interface); err != nil {
			return status.Errorf(codes.Internal, "reset netem: %v", err)
		}
	case "link-state":
		n := r.Nodes[f.Node]
		if n == nil {
			return status.Error(codes.FailedPrecondition, "fault node no longer exists")
		}
		if err := s.Backend.SetLink(ctx, r.Name, n.Name, n.Control, f.Interface, f.RestoreUp); err != nil {
			return status.Errorf(codes.Internal, "restore link: %v", err)
		}
	default:
		return status.Errorf(codes.Internal, "unknown stored fault kind %q", f.Kind)
	}
	f.Active = false
	if err := s.Store.Save(r); err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	s.event(r, "fault.reverted", f)
	return nil
}

func (s *Server) RunTimeline(ctx context.Context, req *labv1.RunTimelineRequest) (*labv1.TimelineResult, error) {
	started := time.Now()
	created := make([]string, 0)
	rollback := func() {
		for i := len(created) - 1; i >= 0; i-- {
			_, _ = s.RevertFault(context.Background(), &labv1.FaultRef{SessionId: req.GetSessionId(), Id: created[i]})
		}
	}
	for i, action := range req.GetActions() {
		wait := started.Add(time.Duration(action.GetAtMillis()) * time.Millisecond).Sub(time.Now())
		if wait > 0 {
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				rollback()
				return nil, status.Error(codes.Canceled, ctx.Err().Error())
			case <-t.C:
			}
		}
		var err error
		switch a := action.GetAction().(type) {
		case *labv1.TimelineAction_Lifecycle:
			a.Lifecycle.Node.SessionId = req.GetSessionId()
			_, err = s.Lifecycle(ctx, a.Lifecycle)
		case *labv1.TimelineAction_ApplyFault:
			a.ApplyFault.SessionId = req.GetSessionId()
			var f *labv1.Fault
			f, err = s.ApplyFault(ctx, a.ApplyFault)
			if err == nil {
				created = append(created, f.GetId())
			}
		case *labv1.TimelineAction_RevertFault:
			a.RevertFault.SessionId = req.GetSessionId()
			_, err = s.RevertFault(ctx, a.RevertFault)
		case *labv1.TimelineAction_Exec:
			a.Exec.Node.SessionId = req.GetSessionId()
			var result *labv1.ExecResponse
			result, err = s.Exec(ctx, a.Exec)
			if err == nil && result.GetExitCode() != 0 {
				err = status.Errorf(codes.FailedPrecondition, "guest command exited %d", result.GetExitCode())
			}
		case *labv1.TimelineAction_WaitExec:
			err = s.waitExec(ctx, req.GetSessionId(), a.WaitExec)
		default:
			err = status.Error(codes.InvalidArgument, "timeline action is required")
		}
		if err != nil {
			rollback()
			return nil, status.Errorf(codes.Aborted, "timeline action %d: %v", i, err)
		}
	}
	return &labv1.TimelineResult{Completed: int32(len(req.GetActions())), FaultIds: created}, nil
}

func (s *Server) waitExec(ctx context.Context, sessionID string, wait *labv1.WaitExec) error {
	if wait == nil || wait.GetExec() == nil || wait.GetExec().GetNode() == nil || len(wait.GetExec().GetArgv()) == 0 {
		return status.Error(codes.InvalidArgument, "wait_exec requires an executable guest predicate")
	}
	retry := time.Duration(wait.GetRetryMillis()) * time.Millisecond
	if retry <= 0 {
		retry = 100 * time.Millisecond
	}
	timeout := time.Duration(wait.GetTimeoutMillis()) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if timeout > 10*time.Minute {
		return status.Error(codes.InvalidArgument, "wait_exec timeout exceeds 10 minutes")
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	execRequest := wait.GetExec()
	execRequest.Node.SessionId = sessionID
	var last string
	for {
		result, err := s.Exec(waitCtx, execRequest)
		if err == nil {
			last = fmt.Sprintf("exit=%d stdout=%q stderr=%q", result.GetExitCode(), result.GetStdout(), result.GetStderr())
			if result.GetExitCode() == wait.GetExpectedExitCode() &&
				strings.Contains(string(result.GetStdout()), string(wait.GetStdoutContains())) &&
				strings.Contains(string(result.GetStderr()), string(wait.GetStderrContains())) {
				if r, loadErr := s.record(sessionID); loadErr == nil {
					s.event(r, "timeline.predicate.matched", map[string]any{"node": execRequest.GetNode().GetNode()})
				}
				return nil
			}
		} else {
			last = err.Error()
		}
		timer := time.NewTimer(retry)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return status.Errorf(codes.DeadlineExceeded, "wait_exec predicate not satisfied: %s", last)
		case <-timer.C:
		}
	}
}

func (s *Server) Scavenge(ctx context.Context, now time.Time) error {
	records, err := s.Store.List()
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range records {
		if now.Before(r.Expires) {
			continue
		}
		if err := s.Backend.Destroy(ctx, r.TopologyPath); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", r.ID, err))
			continue
		}
		if err := s.Store.Delete(r.ID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Server) CleanupUnkept(ctx context.Context) error {
	records, err := s.Store.List()
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range records {
		if r.Kept {
			continue
		}
		if err := s.Backend.Destroy(ctx, r.TopologyPath); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := s.Store.Delete(r.ID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func topologyBytes(src *labv1.TopologySource) ([]byte, string, error) {
	switch source := src.GetSource().(type) {
	case *labv1.TopologySource_Yaml:
		base := src.GetBaseDirectory()
		if base != "" && !filepath.IsAbs(base) {
			return nil, "", errors.New("topology base_directory must be absolute")
		}
		return source.Yaml, base, nil
	case *labv1.TopologySource_Path:
		if src.GetBaseDirectory() != "" {
			return nil, "", errors.New("topology base_directory is only supported with an in-memory source")
		}
		path, err := filepath.Abs(source.Path)
		if err != nil {
			return nil, "", fmt.Errorf("resolve topology: %w", err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("read topology: %w", err)
		}
		return b, filepath.Dir(path), nil
	default:
		return nil, "", errors.New("topology path or yaml is required")
	}
}

func (s *Server) record(id string) (*session.Record, error) {
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "session id is required")
	}
	r, err := s.Store.Get(id)
	if errors.Is(err, session.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "session not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if r.Nodes == nil {
		r.Nodes = map[string]*session.Node{}
	}
	if r.Faults == nil {
		r.Faults = map[string]*session.Fault{}
	}
	return r, nil
}

func (s *Server) node(ref *labv1.NodeRef) (*session.Record, *session.Node, error) {
	if ref == nil {
		return nil, nil, status.Error(codes.InvalidArgument, "node reference is required")
	}
	r, err := s.record(ref.GetSessionId())
	if err != nil {
		return nil, nil, err
	}
	n := r.Nodes[ref.GetNode()]
	if n == nil {
		return nil, nil, status.Error(codes.NotFound, "node not found")
	}
	return r, n, nil
}

func protoSession(r *session.Record) *labv1.Session {
	nodes := make([]*labv1.Node, 0, len(r.Nodes))
	for _, n := range r.Nodes {
		nodes = append(nodes, &labv1.Node{Name: n.Name, State: n.State})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	return &labv1.Session{Id: r.ID, Name: r.Name, State: r.State, TopologyPath: r.TopologyPath, ArtifactDirectory: r.ArtifactDirectory, ExpiresUnix: r.Expires.Unix(), ResumeToken: r.ResumeToken, Nodes: nodes}
}

func randomID(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func diskPlan(dir string, nodes map[string]*labv1.NodeExtension) (map[string][]string, map[string][]session.Disk, error) {
	binds := map[string][]string{}
	out := map[string][]session.Disk{}
	for nodeName, ext := range nodes {
		if len(ext.GetDisks()) != 0 && ext.GetControl() != "qga" {
			return nil, nil, fmt.Errorf("node %q disks require control: qga", nodeName)
		}
		seen := map[string]bool{}
		for _, disk := range ext.GetDisks() {
			if !diskNamePattern.MatchString(disk.GetName()) || seen[disk.GetName()] {
				return nil, nil, fmt.Errorf("node %q has invalid or duplicate disk name %q", nodeName, disk.GetName())
			}
			if disk.GetSizeBytes() == 0 || disk.GetSizeBytes() > 1<<40 {
				return nil, nil, fmt.Errorf("node %q disk %q size must be between 1 byte and 1 TiB", nodeName, disk.GetName())
			}
			seen[disk.GetName()] = true
			path := filepath.Join(dir, "disks", nodeName, disk.GetName()+".raw")
			out[nodeName] = append(out[nodeName], session.Disk{Name: disk.GetName(), Path: path, SizeBytes: disk.GetSizeBytes()})
			binds[nodeName] = append(binds[nodeName], path+":/labcontainers-disks/"+disk.GetName()+".raw")
		}
	}
	return binds, out, nil
}

func createDisks(disks map[string][]session.Disk) error {
	for _, nodeDisks := range disks {
		if err := resetDisks(nodeDisks); err != nil {
			return err
		}
	}
	return nil
}

func resetDisks(disks []session.Disk) error {
	for _, disk := range disks {
		if err := os.MkdirAll(filepath.Dir(disk.Path), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(disk.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		if err := f.Truncate(int64(disk.SizeBytes)); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

type bootstrapFile struct {
	Format, Path string
	Value        []byte
}

func bootstrapPlan(dir string, nodes map[string]*labv1.NodeExtension) (map[string]bootstrapFile, map[string][]string, error) {
	out, binds := map[string]bootstrapFile{}, map[string][]string{}
	for nodeName, ext := range nodes {
		data := ext.GetBootstrap()
		if data == nil || len(data.GetValue()) == 0 {
			continue
		}
		if ext.GetControl() != "qga" {
			return nil, nil, fmt.Errorf("node %q bootstrap requires control: qga", nodeName)
		}
		value := bootstrap.Data{Format: data.GetFormat(), Value: data.GetValue()}
		if err := value.Validate(); err != nil {
			return nil, nil, fmt.Errorf("node %q: %w", nodeName, err)
		}
		path := filepath.Join(dir, "bootstrap", nodeName, "userdata")
		target, err := bootstrapTarget(value.Format)
		if err != nil {
			return nil, nil, err
		}
		out[nodeName] = bootstrapFile{Format: value.Format, Path: path, Value: value.Value}
		binds[nodeName] = append(binds[nodeName], path+":"+target+":ro")
	}
	return out, binds, nil
}

func bootstrapTarget(format string) (string, error) {
	switch format {
	case bootstrap.CloudConfig:
		return "/extra-userdata.yaml", nil
	case bootstrap.Ignition:
		return "/labcontainers-bootstrap/ignition.json", nil
	case bootstrap.PowerShell:
		return "/labcontainers-bootstrap/userdata.ps1", nil
	default:
		return "", fmt.Errorf("unsupported bootstrap format %q", format)
	}
}

func writeBootstraps(files map[string]bootstrapFile) error {
	for _, file := range files {
		if err := os.MkdirAll(filepath.Dir(file.Path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(file.Path, file.Value, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) configureBootstrap(r *session.Record, n *session.Node, data *labv1.BootstrapData) error {
	value := bootstrap.Data{Format: data.GetFormat(), Value: data.GetValue()}
	if err := value.Validate(); err != nil {
		return err
	}
	if n.Control != "qga" {
		return fmt.Errorf("node %q bootstrap requires control: qga", n.Name)
	}
	if n.BootstrapFormat != "" && n.BootstrapFormat != value.Format {
		return fmt.Errorf("node %q bootstrap format cannot change from %q to %q", n.Name, n.BootstrapFormat, value.Format)
	}
	if n.BootstrapPath == "" {
		n.BootstrapPath = filepath.Join(s.Store.SessionDir(r.ID), "bootstrap", n.Name, "userdata")
	}
	target, err := bootstrapTarget(value.Format)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(n.BootstrapPath), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(n.BootstrapPath, value.Value, 0o600); err != nil {
		return err
	}
	if n.BootstrapFormat == "" {
		raw, err := os.ReadFile(r.TopologyPath)
		if err != nil {
			return err
		}
		raw, err = spec.AddNodeBind(raw, n.Name, n.BootstrapPath+":"+target+":ro")
		if err != nil {
			return err
		}
		if err := os.WriteFile(r.TopologyPath, raw, 0o600); err != nil {
			return err
		}
	}
	n.BootstrapFormat = value.Format
	return nil
}

func (s *Server) event(r *session.Record, kind string, detail any) {
	if r.ArtifactDirectory == "" {
		return
	}
	_ = os.MkdirAll(r.ArtifactDirectory, 0o700)
	f, err := os.OpenFile(filepath.Join(r.ArtifactDirectory, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(map[string]any{"time": time.Now().UTC(), "session": r.ID, "event": kind, "detail": detail})
}
