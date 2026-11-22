package phase

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAFailedPhaseStopsTheRunButKeepsItsEvidence(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "evidence")
	var order []string
	boom := errors.New("node never became ready")
	step := func(name string, err error) Phase {
		return Phase{Name: name, Run: func(_ context.Context, evidence string) error {
			order = append(order, name)
			return errors.Join(err, os.WriteFile(filepath.Join(evidence, "log.txt"), []byte(name), 0o600))
		}, Collect: func(ctx context.Context, evidence string) error {
			order = append(order, "collect-"+name)
			if ctx.Err() != nil {
				t.Error("evidence collection inherited an expired deadline")
			}
			return os.WriteFile(filepath.Join(evidence, "state.txt"), []byte("state"), 0o600)
		}}
	}
	var started []string
	runner := Runner{Evidence: dir, OnStart: func(name string, _ time.Time) { started = append(started, name) }}
	ctx, cancel := context.WithCancel(context.Background())
	failing := step("upgrade", boom)
	run := failing.Run
	failing.Run = func(ctx context.Context, evidence string) error { cancel(); return run(ctx, evidence) }
	records, err := runner.Run(ctx, step("baseline", nil), failing, step("verify", nil))
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "phase upgrade") {
		t.Fatalf("error: %v", err)
	}
	if strings.Join(order, ",") != "baseline,collect-baseline,upgrade,collect-upgrade" || strings.Join(started, ",") != "baseline,upgrade" {
		t.Fatalf("order: %v started: %v", order, started)
	}
	if records[0].Status != Passed || records[1].Status != Failed || records[2].Status != NotRun || records[1].Error == "" {
		t.Fatalf("records: %+v", records)
	}
	if _, err := os.Stat(filepath.Join(dir, "02-upgrade", "state.txt")); err != nil {
		t.Fatal("the failed phase's evidence was not collected:", err)
	}
	var written []Record
	data, err := os.ReadFile(filepath.Join(dir, "phases.json"))
	if err != nil || json.Unmarshal(data, &written) != nil || len(written) != 3 || written[2].Status != NotRun {
		t.Fatalf("phases.json: %s %v", data, err)
	}
}

func TestProvisioningIsSkippedWhenRestoredFromACheckpoint(t *testing.T) {
	ran := map[string]bool{}
	phases := []Phase{
		{Name: "build", Provisioning: true, Run: func(context.Context, string) error { ran["build"] = true; return nil }},
		{Name: "upgrade", Run: func(context.Context, string) error { ran["upgrade"] = true; return nil }},
	}
	records, err := Runner{Evidence: t.TempDir(), Restored: true}.Run(context.Background(), phases...)
	if err != nil || ran["build"] || !ran["upgrade"] || records[0].Status != Restored || records[1].Status != Passed {
		t.Fatalf("%v %v %+v", err, ran, records)
	}
	records, err = Runner{Evidence: t.TempDir()}.Run(context.Background(), phases...)
	if err != nil || !ran["build"] || records[0].Status != Passed {
		t.Fatalf("%v %+v", err, records)
	}
}

func TestPhaseTimeoutAndValidation(t *testing.T) {
	slow := Phase{Name: "slow", Timeout: 10 * time.Millisecond, Run: func(ctx context.Context, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	records, err := Runner{Evidence: t.TempDir()}.Run(context.Background(), slow)
	if !errors.Is(err, context.DeadlineExceeded) || records[0].Status != Failed {
		t.Fatalf("%v %+v", err, records)
	}
	ok := func(context.Context, string) error { return nil }
	for _, bad := range [][]Phase{{{Name: "Bad Name", Run: ok}}, {{Name: "a", Run: ok}, {Name: "a", Run: ok}}, {{Name: "a"}}} {
		if _, err := (Runner{Evidence: t.TempDir()}).Run(context.Background(), bad...); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	if _, err := (Runner{Evidence: "relative"}).Run(context.Background(), Phase{Name: "a", Run: ok}); err == nil {
		t.Fatal("accepted a relative evidence directory")
	}
}
