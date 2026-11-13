package probe

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func at(second int) time.Time { return time.Date(2026, 1, 1, 0, 0, second, 0, time.UTC) }

func series(source, path string, pattern string, startSecond int) []Sample {
	var out []Sample
	for i, c := range pattern {
		switch c {
		case '1':
			out = append(out, Sample{Time: at(startSecond + i), Source: source, Path: path, OK: true})
		case '0':
			out = append(out, Sample{Time: at(startSecond + i), Source: source, Path: path, OK: false, Detail: "curl=7"})
		}
	}
	return out
}

func TestEveryOutageIsAWindowWithItsPhases(t *testing.T) {
	samples := append(series("linux", "pod-windows", "111000011101", 0), series("windows", "dns", "1111", 0)...)
	marks := []Mark{{at(8), "restart"}, {at(0), "baseline"}, {at(2), "apply"}}
	report := Outages(samples, marks, 3*time.Second)
	if len(report.Paths) != 2 || report.Paths[0].Path != "pod-windows" {
		t.Fatalf("paths: %+v", report.Paths)
	}
	p := report.Paths[0]
	if p.Samples != 12 || p.Failures != 5 || len(p.Outages) != 2 || len(p.Gaps) != 0 {
		t.Fatalf("accounting: %+v", p)
	}
	first := p.Outages[0]
	if !first.Start.Equal(at(3)) || !first.End.Equal(at(7)) || first.Duration != 4*time.Second || first.AtMost != 5*time.Second || first.Failures != 4 || first.Open || first.Detail != "curl=7" {
		t.Fatalf("first outage: %+v", first)
	}
	if strings.Join(first.Phases, ",") != "apply" {
		t.Fatalf("phases: %v", first.Phases)
	}
	second := p.Outages[1]
	if !second.Start.Equal(at(10)) || second.Failures != 1 || strings.Join(second.Phases, ",") != "restart" {
		t.Fatalf("second outage: %+v", second)
	}
	if p.Down != 5*time.Second {
		t.Fatalf("down: %s", p.Down)
	}
	if clean := report.Paths[1]; len(clean.Outages) != 0 || clean.Failures != 0 {
		t.Fatalf("clean path reported outages: %+v", clean)
	}
	outages, _ := report.Within(at(8), at(11))
	if len(outages) != 1 || !outages[0].Start.Equal(at(10)) {
		t.Fatalf("within: %+v", outages)
	}
	if text := report.Text(); !strings.Contains(text, "linux -> pod-windows: 12 samples, 5 failed, 2 outages (down 5s)") || !strings.Contains(text, "phases=apply") {
		t.Fatal(text)
	}
}

func TestSilenceIsAGapAndATrailingFailureStaysOpen(t *testing.T) {
	samples := append(series("windows", "wan", "11", 0), series("windows", "wan", "100", 30)...)
	report := Outages(samples, []Mark{{at(0), "baseline"}, {at(10), "node-restart"}}, 5*time.Second)
	p := report.Paths[0]
	if len(p.Gaps) != 1 || p.Gaps[0].Duration != 29*time.Second || strings.Join(p.Gaps[0].Phases, ",") != "baseline,node-restart" {
		t.Fatalf("gap: %+v", p.Gaps)
	}
	if len(p.Outages) != 1 || !p.Outages[0].Open || p.Outages[0].Failures != 2 || p.Outages[0].AtMost != 0 {
		t.Fatalf("open outage: %+v", p.Outages)
	}
	if !strings.Contains(report.Text(), "STILL DOWN") {
		t.Fatal("an unrecovered path must be called out")
	}
}

func TestParseToleratesOnlyATruncatedLastLine(t *testing.T) {
	good := `{"t":"2026-01-01T00:00:00Z","from":"linux","path":"dns","ok":true,"detail":"answer=10.0.0.1"}` + "\n"
	samples, err := Parse(strings.NewReader("\xef\xbb\xbf" + good + good + `{"t":"2026-01-01T00:00:02Z","from":"li`))
	if err != nil || len(samples) != 2 || !samples[1].OK || samples[0].Detail != "answer=10.0.0.1" {
		t.Fatalf("%v %+v", err, samples)
	}
	if _, err := Parse(strings.NewReader(good + "garbage\n" + good)); err == nil {
		t.Fatal("accepted a malformed complete line")
	}
	if _, err := Parse(strings.NewReader(`{"t":"2026-01-01T00:00:00Z","from":"","path":"dns","ok":true}` + "\n")); err == nil {
		t.Fatal("accepted a sample without a source")
	}
}

func TestLoopsRejectAmbiguousOrInjectableTargets(t *testing.T) {
	base := Loop{Source: "linux", Directory: "/var/log/probe", Interval: time.Second, Timeout: 2 * time.Second}
	for _, targets := range [][]Target{
		nil,
		{{Name: "Bad Name", URL: "http://10.0.0.1/"}},
		{{Name: "a", URL: "http://10.0.0.1/'; rm -rf /"}},
		{{Name: "a", URL: "ftp://10.0.0.1/"}},
		{{Name: "a", URL: "http://10.0.0.1/", DNSName: "x"}},
		{{Name: "a", DNSName: "svc.cluster.local"}},
		{{Name: "a", URL: "http://10.0.0.1/"}, {Name: "a", URL: "http://10.0.0.2/"}},
	} {
		loop := base
		loop.Targets = targets
		if _, err := loop.Shell(); err == nil {
			t.Fatalf("accepted %+v", targets)
		}
		if _, err := loop.PowerShell(); err == nil {
			t.Fatalf("accepted %+v", targets)
		}
	}
	loop := base
	loop.Directory, loop.Source = `C:\probe`, "windows"
	loop.Targets = []Target{{Name: "pod-linux", URL: "http://10.0.0.1:8080/", Status: 200}, {Name: "dns", DNSName: "svc.default.svc.cluster.local", DNSServer: "10.96.0.10"}}
	script, err := loop.PowerShell()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`'C:\probe\windows-pod-linux.jsonl'`, `'dns','dns','svc.default.svc.cluster.local','10.96.0.10'`, `'http','http://10.0.0.1:8080/','200'`} {
		if !strings.Contains(script, want) {
			t.Fatalf("missing %s in\n%s", want, script)
		}
	}
	if pwsh, err := exec.LookPath("pwsh"); err == nil {
		check := exec.Command(pwsh, "-NoProfile", "-Command", "$e=$null; [void][System.Management.Automation.Language.Parser]::ParseInput([Console]::In.ReadToEnd(),[ref]$null,[ref]$e); if($e.Count){$e; exit 1}")
		check.Stdin = strings.NewReader(script)
		if out, err := check.CombinedOutput(); err != nil {
			t.Fatalf("generated PowerShell does not parse: %s", out)
		}
	}
}

func TestShellLoopRecordsARealOutage(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl required")
	}
	var down atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	loop := Loop{Source: "host", Directory: dir, Interval: time.Second, Timeout: time.Second, Targets: []Target{
		{Name: "web", URL: server.URL + "/", Status: 200},
		{Name: "any", URL: server.URL + "/"},
	}}
	script, err := loop.Shell()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", script)
	cmd.SysProcAttr = processGroup()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer killGroup(cmd)
	time.Sleep(2500 * time.Millisecond)
	down.Store(true)
	time.Sleep(2500 * time.Millisecond)
	down.Store(false)
	time.Sleep(2500 * time.Millisecond)
	killGroup(cmd)
	var samples []Sample
	for _, target := range []string{"web", "any"} {
		f, err := os.Open(filepath.Join(dir, "host-"+target+".jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := Parse(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, parsed...)
	}
	report := Outages(samples, nil, 5*time.Second)
	if len(report.Paths) != 2 {
		t.Fatalf("paths: %+v", report)
	}
	for _, p := range report.Paths {
		switch p.Path {
		case "web":
			if len(p.Outages) != 1 || p.Outages[0].Open || p.Outages[0].Failures < 1 || !strings.Contains(p.Outages[0].Detail, "http=503") {
				t.Fatalf("status-checked path: %+v", p)
			}
		case "any":
			if len(p.Outages) != 0 || p.Samples < 5 {
				t.Fatalf("a path that accepts any response saw an outage: %+v", p)
			}
		}
	}
}
