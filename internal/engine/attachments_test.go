package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type attachmentRunner struct {
	attached, killed, restored      bool
	failSave, failRestore, peerGone bool
}

func (f *attachmentRunner) Run(_ context.Context, _ io.Reader, argv ...string) (Result, error) {
	s := strings.Join(argv, " ")
	switch {
	case strings.Contains(s, "--format") && strings.HasPrefix(s, "docker ps"):
		// Docker's quiet flag overrides --format and drops the node label.
		if strings.Contains(s, "docker ps -q ") {
			return Result{Stdout: []byte("peer-id\n")}, nil
		}
		if f.peerGone {
			return Result{}, nil
		}
		return Result{Stdout: []byte("peer-id\tswitch\n")}, nil
	case strings.Contains(s, "docker exec peer-id sh"):
		if f.failSave {
			return Result{}, errors.New("cannot inspect peer")
		}
		if f.attached {
			return Result{Stdout: []byte("eth3 br0\n")}, nil
		}
	case strings.Contains(s, "docker ps -q"):
		return Result{Stdout: []byte("vm-id\n")}, nil
	case strings.Contains(s, "docker kill"):
		f.attached = false
		f.killed = true
	case s == "docker exec peer-id ip link set dev eth3 master br0":
		if f.failRestore {
			return Result{ExitCode: 1}, nil
		}
		f.attached = true
		f.restored = true
	}
	return Result{}, nil
}

func TestAttachmentInspectionFailurePreventsCrash(t *testing.T) {
	f := &attachmentRunner{attached: true, failSave: true}
	c := &Containerlab{Runner: f}
	if err := c.Lifecycle(context.Background(), filepath.Join(t.TempDir(), "topology.yml"), "vm", "crash"); err == nil {
		t.Fatal("accepted failed attachment capture")
	}
	if f.killed {
		t.Fatal("crashed without capturing recovery state")
	}
}

func TestAttachmentRestoreFailsClosedAndCanRetry(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmtBool(missing), func(t *testing.T) {
			f := &attachmentRunner{attached: true}
			c := &Containerlab{Runner: f}
			topology := filepath.Join(t.TempDir(), "topology.yml")
			if err := c.Lifecycle(context.Background(), topology, "vm", "crash"); err != nil {
				t.Fatal(err)
			}
			f.peerGone, f.failRestore = missing, !missing
			if err := c.Lifecycle(context.Background(), topology, "vm", "start"); err == nil {
				t.Fatal("accepted failed restore")
			}
			if f.restored {
				t.Fatal("unexpected restore outside validated scope")
			}
			if _, err := os.Stat(attachmentPath(topology, "vm")); err != nil {
				t.Fatal("lost retry journal", err)
			}
			f.peerGone, f.failRestore = false, false
			if err := c.Lifecycle(context.Background(), topology, "vm", "start"); err != nil {
				t.Fatal(err)
			}
			if !f.restored {
				t.Fatal("retry did not restore")
			}
			if _, err := os.Stat(attachmentPath(topology, "vm")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("completed journal not removed", err)
			}
		})
	}
}

func fmtBool(missing bool) string {
	if missing {
		return "peer-outside-session"
	}
	return "restore-command-failed"
}

func TestCrashStartPreservesPeerBridgeAttachment(t *testing.T) {
	f := &attachmentRunner{attached: true}
	c := &Containerlab{Runner: f}
	topology := filepath.Join(t.TempDir(), "topology.yml")
	if err := c.Lifecycle(context.Background(), topology, "vm", "crash"); err != nil {
		t.Fatal(err)
	}
	if !f.killed || f.attached {
		t.Fatal("test did not simulate cable removal")
	}
	// A new backend instance must still recover persisted attachment state.
	c = &Containerlab{Runner: f}
	if err := c.Lifecycle(context.Background(), topology, "vm", "start"); err != nil {
		t.Fatal(err)
	}
	if !f.attached || !f.restored {
		t.Fatal("Start lost the peer bridge attachment")
	}
}
