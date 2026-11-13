package components

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const digestA = "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const digestB = "sha256:" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestDiffListsExactlyWhatATransitionChanges(t *testing.T) {
	from := Set{Name: "from", Components: map[string]Component{
		"kubernetes": {Version: "1.36.4"}, "node": {Reference: "example.test/node@" + digestA},
		"pause": {Reference: "example.test/pause@" + digestA}, "retired": {Version: "1.0.0"},
	}}
	to := Set{Name: "to", Components: map[string]Component{
		"kubernetes": {Version: "1.37.2"}, "node": {Reference: "example.test/node@" + digestB},
		"pause": {Reference: "example.test/pause@" + digestA}, "added": {Version: "2.0.0"},
	}}
	if err := from.Validate(); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range Diff(from, to) {
		got = append(got, c.Name+":"+c.Was+"->"+c.Becomes)
	}
	want := "added:->2.0.0;kubernetes:1.36.4->1.37.2;node:example.test/node@" + digestA + "->example.test/node@" + digestB + ";retired:1.0.0->"
	if strings.Join(got, ";") != want {
		t.Fatalf("diff:\n%s\nwant:\n%s", strings.Join(got, ";"), want)
	}
	node, err := to.Get("node")
	if err != nil || node.Digest() != digestB || node.Repository() != "example.test/node" {
		t.Fatalf("%+v %v", node, err)
	}
	if _, err := to.Get("retired"); err == nil {
		t.Fatal("returned a component the set does not have")
	}
}

func TestSetsCannotFloat(t *testing.T) {
	for name, c := range map[string]Component{
		"unpinned":     {},
		"tag":          {Reference: "example.test/node:v1"},
		"floating":     {Version: "latest"},
		"bad hash":     {Version: "1", SHA256: "abc"},
		"unhashedfile": {Version: "1", File: "k0s"},
	} {
		if err := (Set{Name: "s", Components: map[string]Component{"c": c}}).Validate(); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	if err := (Set{Components: map[string]Component{"c": {Version: "1"}}}).Validate(); err == nil {
		t.Fatal("accepted an unnamed set")
	}
}

func TestReadRequiresThePinnedBytes(t *testing.T) {
	body := []byte(`{"name":"from","components":{"kubernetes":{"version":"1.36.4"}}}`)
	path := filepath.Join(t.TempDir(), "from.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(body))
	set, err := Read(context.Background(), path, sum)
	if err != nil || set.Components["kubernetes"].Version != "1.36.4" {
		t.Fatalf("%+v %v", set, err)
	}
	if _, err := Read(context.Background(), path, strings.Repeat("0", 64)); err == nil {
		t.Fatal("read a set whose content was not the pinned one")
	}
	if err := os.WriteFile(path, []byte(`{"name":"from","components":{"k":{"version":"1"}},"extra":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	extra, _ := os.ReadFile(path)
	if _, err := Read(context.Background(), path, fmt.Sprintf("%x", sha256.Sum256(extra))); err == nil {
		t.Fatal("accepted unknown fields")
	}
}
