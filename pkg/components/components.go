// Package components names the versioned set of software a lab runs, so a
// test can state what it starts from, what it moves to, and exactly which
// components the move changes.
package components

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/appmana/labcontainers/pkg/artifact"
)

// Component is one pinned piece of software. A component is identified by a
// version, an image reference pinned by digest, or both; SHA256 pins a file.
type Component struct {
	Version   string `json:"version,omitempty"`
	Reference string `json:"reference,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	// File is the artifact's name where the caller staged it.
	File string `json:"file,omitempty"`
	// URL is where the deployment's own tooling downloads it from.
	URL string `json:"url,omitempty"`
}

// Set is everything running at one point of a lab's life.
type Set struct {
	Name       string               `json:"name"`
	Components map[string]Component `json:"components"`
}

var (
	digestRef = regexp.MustCompile(`^[a-z0-9][a-z0-9./_-]*@sha256:[0-9a-f]{64}$`)
	sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Validate rejects anything that could float: a component must carry a
// version or a digest-pinned reference, and a staged file must be hashed.
func (s Set) Validate() error {
	if strings.TrimSpace(s.Name) == "" || len(s.Components) == 0 {
		return fmt.Errorf("a component set needs a name and components")
	}
	for name, c := range s.Components {
		if c.Version == "" && c.Reference == "" {
			return fmt.Errorf("%s: %s has neither a version nor a reference", s.Name, name)
		}
		if c.Reference != "" && !digestRef.MatchString(c.Reference) {
			return fmt.Errorf("%s: %s reference %q is not pinned by digest", s.Name, name, c.Reference)
		}
		if c.SHA256 != "" && !sha256Hex.MatchString(c.SHA256) {
			return fmt.Errorf("%s: %s has an invalid SHA-256", s.Name, name)
		}
		if c.File != "" && c.SHA256 == "" {
			return fmt.Errorf("%s: %s names file %s without its SHA-256", s.Name, name, c.File)
		}
		for _, floating := range []string{"latest", "stable", "main", "master"} {
			if strings.EqualFold(c.Version, floating) {
				return fmt.Errorf("%s: %s version %q floats", s.Name, name, c.Version)
			}
		}
	}
	return nil
}

// Get returns a component the caller cannot do without.
func (s Set) Get(name string) (Component, error) {
	c, ok := s.Components[name]
	if !ok {
		return c, fmt.Errorf("component set %s has no %s", s.Name, name)
	}
	return c, nil
}

// Digest is the reference's sha256:... part.
func (c Component) Digest() string {
	if i := strings.Index(c.Reference, "@"); i >= 0 {
		return c.Reference[i+1:]
	}
	return ""
}

// Repository is the reference without its digest.
func (c Component) Repository() string {
	if i := strings.Index(c.Reference, "@"); i >= 0 {
		return c.Reference[:i]
	}
	return c.Reference
}

// Read loads a set from a file whose content hash the caller pinned.
func Read(ctx context.Context, path, sha256 string) (Set, error) {
	var s Set
	data, err := artifact.ReadFile(ctx, path, sha256)
	if err != nil {
		return s, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return s, fmt.Errorf("decode component set %s: %w", path, err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return s, fmt.Errorf("component set %s must be one JSON object", path)
	}
	return s, s.Validate()
}

// Change is one component's move between two sets. A zero From is an
// addition and a zero To a removal.
type Change struct {
	Name     string    `json:"name"`
	From, To Component `json:"-"`
	Was      string    `json:"from"`
	Becomes  string    `json:"to"`
}

func describe(c Component) string {
	parts := []string{}
	for _, v := range []string{c.Version, c.Reference} {
		if v != "" {
			parts = append(parts, v)
		}
	}
	if len(parts) == 0 && c.SHA256 != "" {
		parts = append(parts, "sha256:"+c.SHA256)
	}
	return strings.Join(parts, " ")
}

// Diff lists every component whose pin differs between from and to, by name.
// Components equal in both sets are what the transition must leave alone.
func Diff(from, to Set) []Change {
	names := map[string]bool{}
	for name := range from.Components {
		names[name] = true
	}
	for name := range to.Components {
		names[name] = true
	}
	var changes []Change
	for name := range names {
		a, b := from.Components[name], to.Components[name]
		if a.Version == b.Version && a.Reference == b.Reference && a.SHA256 == b.SHA256 {
			continue
		}
		changes = append(changes, Change{Name: name, From: a, To: b, Was: describe(a), Becomes: describe(b)})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Name < changes[j].Name })
	return changes
}
