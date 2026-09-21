package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

var ErrNotFound = errors.New("session not found")

type Node struct {
	Name            string `json:"name"`
	Control         string `json:"control,omitempty"`
	State           string `json:"state"`
	Disks           []Disk `json:"disks,omitempty"`
	BootstrapFormat string `json:"bootstrapFormat,omitempty"`
	BootstrapPath   string `json:"bootstrapPath,omitempty"`
}

type Disk struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	SizeBytes uint64 `json:"sizeBytes"`
}

type Fault struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Node      string `json:"node,omitempty"`
	Interface string `json:"interface,omitempty"`
	Active    bool   `json:"active"`
	RestoreUp bool   `json:"restoreUp,omitempty"`
}

type Record struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	State             string            `json:"state"`
	TopologyPath      string            `json:"topologyPath"`
	ArtifactDirectory string            `json:"artifactDirectory"`
	Expires           time.Time         `json:"expires"`
	ResumeToken       string            `json:"resumeToken,omitempty"`
	Kept              bool              `json:"kept,omitempty"`
	Nodes             map[string]*Node  `json:"nodes"`
	Faults            map[string]*Fault `json:"faults,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
}

type Store struct {
	root string
	mu   sync.Mutex
}

func DefaultRoot() (string, error) {
	if root := os.Getenv("LABCONTAINERS_STATE_DIR"); root != "" {
		return root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "labcontainers"), nil
}

func NewStore(root string) (*Store, error) {
	if root == "" {
		var err error
		root, err = DefaultRoot()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o700); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (s *Store) Root() string                { return s.root }
func (s *Store) SessionDir(id string) string { return filepath.Join(s.root, "sessions", id) }

func (s *Store) Create(r *Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := s.SessionDir(r.ID)
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(dir, "artifacts"), 0o700); err != nil {
		return err
	}
	return s.writeLocked(r)
}

func (s *Store) Save(r *Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeLocked(r)
}

func (s *Store) writeLocked(r *Record) error {
	dir := s.SessionDir(r.ID)
	tmp, err := os.CreateTemp(dir, ".session-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dir, "session.json"))
}

func (s *Store) Get(id string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(filepath.Join(s.SessionDir(id), "session.json"))
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("decode session %s: %w", id, err)
	}
	return &r, nil
}

func (s *Store) List() ([]*Record, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "sessions"))
	if err != nil {
		return nil, err
	}
	var out []*Record
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		r, err := s.Get(entry.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.RemoveAll(s.SessionDir(id))
}
