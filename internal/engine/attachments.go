package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type peerAttachment struct {
	Container string
	Interface string
	Master    string
}

func attachmentPath(topology, node string) string {
	return filepath.Join(filepath.Dir(topology), fmt.Sprintf("attachments-%x.json", sha256.Sum256([]byte(topology+"\x00"+node))))
}

// Only inspect Linux containers owned by this exact session. Generic VM guest
// networking is not configured here, and host bridges are never touched.
func (c *Containerlab) linuxPeers(ctx context.Context, topology, node string) (map[string]bool, error) {
	r, err := checked(ctx, c.Runner, nil, "docker", "ps", "--no-trunc", "--filter", "label=clab-topo-file="+topology,
		"--filter", "label=clab-node-kind=linux", "--format", "{{.ID}}\t{{.Label \"clab-node-name\"}}")
	if err != nil {
		return nil, err
	}
	peers := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(r.Stdout)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid session peer inspection %q", line)
		}
		if fields[1] != node {
			peers[fields[0]] = true
		}
	}
	return peers, nil
}

// Save bridge memberships before runtime teardown deletes the peer veths.
// Keep the journal through failed starts and daemon recreation; never replace
// pending recovery state with a snapshot of already disconnected ports.
func (c *Containerlab) saveAttachments(ctx context.Context, topology, node string) error {
	path := attachmentPath(topology, node)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	peers, err := c.linuxPeers(ctx, topology, node)
	if err != nil {
		return err
	}
	var saved []peerAttachment
	for id := range peers {
		r, err := checked(ctx, c.Runner, nil, "docker", "exec", id, "sh", "-ec",
			`for p in /sys/class/net/*; do if [ -L "$p/master" ]; then m=$(readlink "$p/master"); printf '%s %s\n' "${p##*/}" "${m##*/}"; fi; done`)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(strings.TrimSpace(string(r.Stdout)), "\n") {
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) != 2 {
				return fmt.Errorf("invalid attachment %q", line)
			}
			saved = append(saved, peerAttachment{id, fields[0], fields[1]})
		}
	}
	if len(saved) == 0 {
		return nil
	}
	data, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".attachments-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp.Name(), path)
}

func (c *Containerlab) restoreAttachments(ctx context.Context, topology, node string) error {
	path := attachmentPath(topology, node)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var saved []peerAttachment
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}
	peers, err := c.linuxPeers(ctx, topology, node)
	if err != nil {
		return err
	}
	for _, a := range saved {
		if !peers[a.Container] {
			return fmt.Errorf("attachment peer %s is no longer a running session member", a.Container)
		}
		if _, err := checked(ctx, c.Runner, nil, "docker", "exec", a.Container, "ip", "link", "set", "dev", a.Interface, "master", a.Master); err != nil {
			return err
		}
	}
	return os.Remove(path)
}
