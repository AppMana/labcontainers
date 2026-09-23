package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
)

// TestLiveWindows exercises the installed Windows guest, not a cross-build.
func TestLiveWindows(t *testing.T) {
	if os.Getenv("LABCONTAINERS_WINDOWS_LIVE") != "1" {
		t.Skip("set LABCONTAINERS_WINDOWS_LIVE=1")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	c, err := Launch(ctx, Options{LabdPath: filepath.Join(root, "bin", "labd")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}()
	img := os.Getenv("LABCONTAINERS_WINDOWS_IMAGE")
	if img == "" {
		img = "labcontainers/windows-server-2022:latest"
	}
	lab, err := c.Start(ctx, &labv1.LabSpec{Topology: vmTopology(t, img), Nodes: map[string]*labv1.NodeExtension{"vm": {Control: "qga", Disks: []*labv1.Disk{{Name: "volume", SizeBytes: 1 << 30}}}}}, 18*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	node := lab.Node("vm")
	ps := func(script string) *labv1.ExecResponse {
		t.Helper()
		result, err := node.Exec(ctx, `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, "-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference='Stop'; "+script)
		if err != nil {
			t.Fatal(err)
		}
		if result.GetExitCode() != 0 {
			t.Fatalf("PowerShell exit %d: %s %s", result.GetExitCode(), result.GetStdout(), result.GetStderr())
		}
		return result
	}
	ref := &labv1.NodeRef{Node: "vm"}
	_, err = lab.RunTimeline(ctx, &labv1.TimelineAction{Action: &labv1.TimelineAction_WaitExec{WaitExec: &labv1.WaitExec{Exec: &labv1.ExecRequest{Node: ref, Argv: []string{`C:\Windows\System32\cmd.exe`, "/c", "ver"}, TimeoutMillis: 10000}, TimeoutMillis: 600000, RetryMillis: 2000, StdoutContains: []byte("Windows")}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Windows guest ready")
	beforeBoot := string(ps(`(Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToUniversalTime().ToString("O")`).GetStdout())
	ps(`$d=@(Get-Disk | Where-Object { $_.SerialNumber.Trim() -eq 'lc-volume' }); if($d.Count -ne 1 -or $d[0].PartitionStyle -ne 'RAW'){throw 'expected exactly one fresh lab disk'}; $d[0] | Initialize-Disk -PartitionStyle GPT -PassThru | New-Partition -UseMaximumSize -DriveLetter V | Format-Volume -FileSystem NTFS -Confirm:$false | Out-Null`)
	content := []byte{0, 10, 13, 255, 128, 42}
	if err := node.Put(ctx, `V:\uploaded.bin`, 0600, content); err != nil {
		t.Fatal(err)
	}
	ps(`$b=[IO.File]::ReadAllBytes('V:\uploaded.bin'); if([Convert]::ToBase64String($b) -ne 'AAoN/4Aq'){throw 'binary upload mismatch'}; $f=[IO.File]::Open('V:\marker',[IO.FileMode]::Create); try{$b=[Text.Encoding]::UTF8.GetBytes('durable-windows'); $f.Write($b,0,$b.Length); $f.Flush($true)}finally{$f.Dispose()}`)
	if err := node.Crash(ctx); err != nil {
		t.Fatal(err)
	}
	recoverVM(t, ctx, lab, "vm")
	_, err = lab.RunTimeline(ctx, &labv1.TimelineAction{Action: &labv1.TimelineAction_WaitExec{WaitExec: &labv1.WaitExec{Exec: &labv1.ExecRequest{Node: ref, Argv: []string{`C:\Windows\System32\cmd.exe`, "/c", "ver"}, TimeoutMillis: 10000}, TimeoutMillis: 600000, RetryMillis: 2000, StdoutContains: []byte("Windows")}}})
	if err != nil {
		t.Fatal(err)
	}
	ps(`$d=@(Get-Disk | Where-Object { $_.SerialNumber.Trim() -eq 'lc-volume' }); if($d.Count -ne 1){throw 'disk missing'}; $d[0] | Set-Disk -IsOffline $false; $d[0] | Set-Disk -IsReadOnly $false; $p=Get-Partition -DiskNumber $d[0].Number | Where-Object Type -eq 'Basic'; if($p.DriveLetter -ne 'V'){$p | Set-Partition -NewDriveLetter V}; if([IO.File]::ReadAllText('V:\marker') -ne 'durable-windows'){throw 'durable marker lost'}`)
	afterBoot := string(ps(`(Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToUniversalTime().ToString("O")`).GetStdout())
	if beforeBoot == afterBoot {
		t.Fatal("VM boot identity did not change after crash")
	}
	t.Log("Windows NTFS marker survived abrupt VM power cycle")
}
