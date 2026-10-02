// Package vyos configures an explicit VyOS guest through its native config
// session API. It does not install an OS, create links, or grant network access.
package vyos

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
)

type Guest interface {
	Pipe(context.Context, io.Reader, ...string) ([]byte, error)
}

type Command struct {
	Operation string   `json:"operation"`
	Path      []string `json:"path"`
}

//go:embed configure.py
var configure string

// Apply commits and saves explicit set/delete paths, using the native VyOS
// validator and transaction. Values remain JSON data, never shell commands.
func Apply(ctx context.Context, guest Guest, commands []Command) error {
	if len(commands) == 0 {
		return fmt.Errorf("explicit VyOS configuration required")
	}
	for _, c := range commands {
		if (c.Operation != "set" && c.Operation != "delete") || len(c.Path) < 2 {
			return fmt.Errorf("invalid or overly broad VyOS command")
		}
		for _, part := range c.Path {
			if part == "" {
				return fmt.Errorf("empty VyOS path component")
			}
		}
	}
	data, err := json.Marshal(commands)
	if err != nil {
		return err
	}
	out, err := guest.Pipe(ctx, bytes.NewReader(data), "runuser", "-u", "vyos", "--", "/usr/bin/python3", "-c", configure)
	if err != nil {
		return fmt.Errorf("VyOS native commit: %w", err)
	}
	if !bytes.Contains(out, []byte("LABCONTAINERS_VYOS_COMMITTED")) {
		return fmt.Errorf("VyOS commit did not acknowledge completion")
	}
	return nil
}
