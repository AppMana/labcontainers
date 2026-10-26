package containerd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"
)

type carryNode struct {
	name     string
	put      map[string]string
	commands [][]string
	failOn   string
}

func (n *carryNode) Name() string { return n.name }
func (n *carryNode) Exec(_ context.Context, argv ...string) ([]byte, error) {
	n.commands = append(n.commands, append([]string(nil), argv...))
	if n.failOn != "" && argv[0] == n.failOn {
		return nil, errors.New("import failed")
	}
	return nil, nil
}
func (n *carryNode) Pipe(context.Context, io.Reader, ...string) ([]byte, error) {
	return nil, errors.New("archives are carried as files, never piped")
}
func (n *carryNode) Put(_ context.Context, src io.Reader, dst string, _ fs.FileMode) error {
	body, err := io.ReadAll(src)
	if n.put == nil {
		n.put = map[string]string{}
	}
	n.put[dst] = string(body)
	return err
}

// Each node gets the archive as a file, imports it with the caller's
// command, and removes it, whatever the import did.
func TestAnArchiveIsCarriedAsAFileToEveryNode(t *testing.T) {
	ok := &carryNode{name: "a"}
	failing := &carryNode{name: "b", failOn: "k0s"}
	err := CarryArchive(context.Background(), bytes.NewReader([]byte("archive")), []string{"k0s", "ctr", "images", "import", "-"}, ok, failing)
	if err == nil || !strings.Contains(err.Error(), "b") {
		t.Fatalf("node failure lost: %v", err)
	}
	for _, n := range []*carryNode{ok, failing} {
		if n.put[CarriedArchive] != "archive" {
			t.Fatalf("%s carried %v", n.name, n.put)
		}
		want := [][]string{{"k0s", "ctr", "images", "import", CarriedArchive}, {"rm", "-f", CarriedArchive}}
		if !reflect.DeepEqual(n.commands, want) {
			t.Fatalf("%s ran %v", n.name, n.commands)
		}
	}
}

// An import command that does not read its archive from "-" is refused
// before anything is carried.
func TestAnImportCommandMustReadTheArchiveFromDash(t *testing.T) {
	n := &carryNode{name: "a"}
	if err := CarryArchive(context.Background(), bytes.NewReader(nil), []string{"ctr", "images", "import"}, n); err == nil {
		t.Fatal("accepted an import command without -")
	}
	if len(n.put) != 0 || len(n.commands) != 0 {
		t.Fatalf("touched the node: %v %v", n.put, n.commands)
	}
}
