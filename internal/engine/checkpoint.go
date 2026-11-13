package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// A VM wrapper keeps its guest's writes in a copy-on-write overlay beside the
// image's base disk, inside the container's writable layer.
var overlayPattern = regexp.MustCompile(`^[AC] /([^/]+-overlay\.qcow2)$`)

// stoppedContainer resolves a session node that exists but is not running,
// and the immutable image its overlay is based on.
func (c *Containerlab) stoppedContainer(ctx context.Context, topology, node string) (id, image string, err error) {
	listed, err := checked(ctx, c.Runner, nil, "docker", "ps", "--all", "-q", "--no-trunc",
		"--filter", "label=clab-topo-file="+topology, "--filter", "label=clab-node-name="+node)
	if err != nil {
		return "", "", err
	}
	ids := strings.Fields(string(listed.Stdout))
	if len(ids) != 1 {
		return "", "", fmt.Errorf("node %q resolves to %d containers; expected exactly one", node, len(ids))
	}
	inspected, err := checked(ctx, c.Runner, nil, "docker", "inspect", "--format", "{{.State.Running}} {{.Image}}", ids[0])
	if err != nil {
		return "", "", err
	}
	fields := strings.Fields(string(inspected.Stdout))
	if len(fields) != 2 || fields[1] == "" {
		return "", "", fmt.Errorf("node %q: unexpected container inspection %q", node, inspected.Stdout)
	}
	if fields[0] != "false" {
		return "", "", fmt.Errorf("node %q is running; its disks can only be copied while it is stopped", node)
	}
	return ids[0], fields[1], nil
}

// ExportNode copies a stopped node's root overlays into dir and reports the
// image they depend on. A node that never wrote an overlay exports nothing.
func (c *Containerlab) ExportNode(ctx context.Context, topology, node, dir string) (string, []string, error) {
	id, image, err := c.stoppedContainer(ctx, topology, node)
	if err != nil {
		return "", nil, err
	}
	diff, err := checked(ctx, c.Runner, nil, "docker", "diff", id)
	if err != nil {
		return "", nil, err
	}
	var files []string
	for _, line := range strings.Split(string(diff.Stdout), "\n") {
		if match := overlayPattern.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			files = append(files, match[1])
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return image, nil, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, err
	}
	for _, file := range files {
		if _, err := checked(ctx, c.Runner, nil, "docker", "cp", id+":/"+file, filepath.Join(dir, file)); err != nil {
			return "", nil, fmt.Errorf("copy %s out of node %q: %w", file, node, err)
		}
	}
	return image, files, nil
}

// ImportNode replaces a stopped node's root overlays with saved ones. The
// overlays reference their base disk by path, so the node must run the image
// they were saved from.
func (c *Containerlab) ImportNode(ctx context.Context, topology, node, dir, image string, files []string) error {
	id, current, err := c.stoppedContainer(ctx, topology, node)
	if err != nil {
		return err
	}
	if current != image {
		return fmt.Errorf("node %q runs image %s; the checkpoint was saved from %s", node, current, image)
	}
	for _, file := range files {
		if filepath.Base(file) != file || !strings.HasSuffix(file, "-overlay.qcow2") {
			return fmt.Errorf("node %q: unexpected checkpoint file %q", node, file)
		}
		if _, err := checked(ctx, c.Runner, nil, "docker", "cp", filepath.Join(dir, file), id+":/"+file); err != nil {
			return fmt.Errorf("copy %s into node %q: %w", file, node, err)
		}
	}
	return nil
}

// CopyDisk copies a data disk image, keeping its holes.
func (c *Containerlab) CopyDisk(ctx context.Context, source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	_, err := checked(ctx, c.Runner, nil, "cp", "--sparse=always", "--reflink=auto", "--", source, destination)
	return err
}
