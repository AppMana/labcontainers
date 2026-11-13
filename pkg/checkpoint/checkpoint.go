// Package checkpoint describes a set of stopped lab nodes' disks saved outside
// session state. A checkpoint outlives the session that made it, so a later
// session with the same nodes and images can start from that content instead
// of repeating the provisioning that produced it.
package checkpoint

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	manifestName = "checkpoint.json"
	version      = 1
)

// ErrNone reports a directory that holds no complete checkpoint.
var ErrNone = errors.New("no complete checkpoint")

// Disk is an attached data disk, identified by its declared name and size.
type Disk struct {
	Name      string `json:"name"`
	SizeBytes uint64 `json:"sizeBytes"`
}

// Node is one node's saved content. Root lists the copy-on-write overlays of
// the node's image; they are only meaningful on top of exactly that image.
type Node struct {
	Image string   `json:"image"`
	Root  []string `json:"root,omitempty"`
	Disks []Disk   `json:"disks,omitempty"`
}

type Manifest struct {
	Version int               `json:"version"`
	Created time.Time         `json:"created"`
	Session string            `json:"session"`
	Labels  map[string]string `json:"labels,omitempty"`
	Nodes   map[string]Node   `json:"nodes"`
}

// NodeNames returns the saved nodes in a stable order.
func (m Manifest) NodeNames() []string {
	names := make([]string, 0, len(m.Nodes))
	for name := range m.Nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// RootPath and DiskPath are where a node's files live inside a checkpoint.
func RootPath(dir, node, file string) string { return filepath.Join(dir, node, "root", file) }
func DiskPath(dir, node, disk string) string { return filepath.Join(dir, node, "disks", disk+".raw") }

// Read loads a complete checkpoint's manifest and checks that every file it
// names exists. A directory still being written has no manifest.
func Read(dir string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if os.IsNotExist(err) {
		return m, ErrNone
	}
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("decode checkpoint %s: %w", dir, err)
	}
	if m.Version != version || len(m.Nodes) == 0 {
		return m, fmt.Errorf("checkpoint %s has unsupported version %d or no nodes", dir, m.Version)
	}
	for name, node := range m.Nodes {
		if node.Image == "" {
			return m, fmt.Errorf("checkpoint %s: node %s records no image", dir, name)
		}
		for _, file := range node.Root {
			if _, err := os.Stat(RootPath(dir, name, file)); err != nil {
				return m, fmt.Errorf("checkpoint %s is incomplete: %w", dir, err)
			}
		}
		for _, disk := range node.Disks {
			info, err := os.Stat(DiskPath(dir, name, disk.Name))
			if err != nil {
				return m, fmt.Errorf("checkpoint %s is incomplete: %w", dir, err)
			}
			if uint64(info.Size()) != disk.SizeBytes {
				return m, fmt.Errorf("checkpoint %s: disk %s/%s is %d bytes, recorded %d", dir, name, disk.Name, info.Size(), disk.SizeBytes)
			}
		}
	}
	return m, nil
}

// Write records the manifest last, so its presence marks a complete checkpoint.
func Write(dir string, m Manifest) error {
	m.Version = version
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "."+manifestName)
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, manifestName))
}

// Size is the space the checkpoint's files occupy on disk, which for sparse
// data disks is less than their declared size.
func Size(dir string) (uint64, error) {
	var total uint64
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		total += allocated(info)
		return nil
	})
	return total, err
}
