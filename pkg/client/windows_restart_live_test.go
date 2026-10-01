package client

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/appmana/labcontainers/pkg/windows"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/types"
)

// Exercise the image launcher, not a harness-side disk repair. Repeated native
// stop/start must reuse the same writable disk and preserve flushed guest data.
func TestLiveWindowsRestartPreservesRootDisk(t *testing.T) {
	image := os.Getenv("LABCONTAINERS_WINDOWS_RESTART_IMAGE")
	if image == "" {
		t.Skip("explicit prepared Windows image and KVM required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	c, err := Launch(ctx, Options{LabdPath: os.Getenv("LABCONTAINERS_LABD"), StateDir: os.Getenv("LABCONTAINERS_STATE_DIR")})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	source, err := clab.Source(&core.Config{Topology: &types.Topology{Nodes: map[string]*types.NodeDefinition{
		"windows": {Kind: "generic_vm", Image: image, ImagePullPolicy: "Never", NetworkMode: "none", Env: map[string]string{"QEMU_MEMORY": "4096", "QEMU_SMP": "4", "QEMU_ADDITIONAL_ARGS": "-qmp unix:/run/restart-diagnostic.sock,server=on,wait=off"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	lab, err := c.Start(ctx, &labv1.LabSpec{Topology: source, Nodes: map[string]*labv1.NodeExtension{"windows": {Control: "qga"}}}, 12*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	node := lab.Node("windows")
	t.Logf("artifacts: %s", lab.Artifacts())
	defer func() {
		if !t.Failed() {
			return
		}
		diagnostic, done := context.WithTimeout(context.Background(), time.Minute)
		defer done()
		ids, err := exec.CommandContext(diagnostic, "docker", "ps", "-q", "--filter", "label=labcontainers.appmana.com/session="+lab.ID()).Output()
		if err == nil && len(strings.Fields(string(ids))) == 1 {
			id := strings.TrimSpace(string(ids))
			// Separate local QMP observation avoids competing with vrnetlab's
			// persistent HMP connection; it never changes guest boot settings.
			out, err := exec.CommandContext(diagnostic, "docker", "exec", id, "python3", "-c", `import socket,json
s=socket.socket(socket.AF_UNIX);s.settimeout(10);s.connect('/run/restart-diagnostic.sock');f=s.makefile('rwb');print(f.readline().decode())
for command in [{'execute':'qmp_capabilities'},{'execute':'query-status'},{'execute':'screendump','arguments':{'filename':'/restart-failure.ppm'}}]:
 f.write(json.dumps(command).encode()+b'\n');f.flush()
 while True:
  response=json.loads(f.readline());print(response)
  if 'return' in response or 'error' in response:break
`).CombinedOutput()
			t.Logf("QMP diagnosis: %s %v", out, err)
			out, err = exec.CommandContext(diagnostic, "docker", "cp", id+":/restart-failure.ppm", filepath.Join(lab.Artifacts(), "restart-failure.ppm")).CombinedOutput()
			t.Logf("console capture: %s %v", out, err)
		}
		if err := node.PowerOff(diagnostic); err != nil {
			t.Error(err)
			return
		}
		if err := lab.Keep(diagnostic, 2*time.Hour); err != nil {
			t.Error(err)
		}
		t.Logf("failed VM retained powered off: session=%s socket=%s state=%s", lab.ID(), c.Socket(), c.StateDirectory())
	}()
	ready := func() {
		t.Helper()
		_, err := lab.RunTimeline(ctx, &labv1.TimelineAction{Action: &labv1.TimelineAction_WaitExec{WaitExec: &labv1.WaitExec{
			Exec:          &labv1.ExecRequest{Node: &labv1.NodeRef{Node: "windows"}, Argv: []string{"cmd.exe", "/c", "ver"}, TimeoutMillis: 10000},
			TimeoutMillis: 240000, RetryMillis: 2000, StdoutContains: []byte("Windows"),
		}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	guest := func(script string) string {
		t.Helper()
		r, err := node.ExecWithTimeout(ctx, time.Minute, windows.PowerShellCommand(script)...)
		if err != nil {
			t.Fatal(err)
		}
		if r.ExitCode != 0 {
			t.Fatalf("guest exit %d: %s %s", r.ExitCode, r.Stdout, r.Stderr)
		}
		return strings.TrimSpace(string(r.Stdout))
	}
	disk := func() string {
		t.Helper()
		ids, err := exec.CommandContext(ctx, "docker", "ps", "-q", "--filter", "label=labcontainers.appmana.com/session="+lab.ID()).Output()
		if err != nil || len(strings.Fields(string(ids))) != 1 {
			t.Fatalf("owned wrapper: %s %v", ids, err)
		}
		out, err := exec.CommandContext(ctx, "docker", "exec", strings.TrimSpace(string(ids)), "ps", "-eo", "args").Output()
		if err != nil {
			t.Fatal(err)
		}
		var disks []string
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.HasPrefix(line, "qemu-system-") {
				continue
			}
			for _, arg := range strings.Fields(line) {
				if strings.HasPrefix(arg, "if=ide,file=") {
					disks = append(disks, arg)
				}
			}
		}
		if len(disks) != 1 {
			t.Fatalf("expected one root disk: %q", disks)
		}
		return disks[0]
	}
	ready()
	if err := windows.ConfigureUnattendedRecovery(ctx, node); err != nil {
		t.Fatal(err)
	}
	before := disk()
	want := lab.ID()
	guest(`$f=[IO.File]::Open('C:\restart-canary','CreateNew','Write','None'); try {$b=[Text.Encoding]::UTF8.GetBytes('` + want + `'); $f.Write($b,0,$b.Length); $f.Flush($true)} finally {$f.Dispose()}`)
	for i := 0; i < 2; i++ {
		if err := node.PowerOff(ctx); err != nil {
			t.Fatal(err)
		}
		if err := node.Start(ctx); err != nil {
			t.Fatal(err)
		}
		ready()
		if after := disk(); after != before {
			t.Fatalf("restart %d switched disk: %s -> %s", i+1, before, after)
		}
		if got := guest(`[IO.File]::ReadAllText('C:\restart-canary')`); got != want {
			t.Fatalf("restart %d lost acknowledged canary: %q", i+1, got)
		}
		t.Logf("restart %d: root disk %s and original canary preserved", i+1, before)
	}
}
