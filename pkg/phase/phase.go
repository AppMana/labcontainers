// Package phase runs a lab scenario as named phases that share one long-lived
// lab. Each phase gets its own evidence directory and a record of when it ran
// and how it ended; a failed phase stops the run, because later phases would
// only measure the damage. A phase can be marked as provisioning: a run that
// starts from a checkpoint taken after it records the phase as restored
// instead of running it again.
package phase

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Phase is one named step.
type Phase struct {
	Name string
	Run  func(ctx context.Context, evidence string) error
	// Collect gathers evidence after Run, whether or not it failed, with its
	// own deadline. Its failure is recorded but does not fail the phase.
	Collect func(ctx context.Context, evidence string) error
	// Provisioning phases build the state a checkpoint holds.
	Provisioning bool
	// Timeout bounds Run; zero leaves the run's context in charge.
	Timeout time.Duration
}

// Record is what one phase did.
type Record struct {
	Name     string    `json:"name"`
	Status   string    `json:"status"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	Seconds  float64   `json:"seconds"`
	Evidence string    `json:"evidence"`
	Error    string    `json:"error,omitempty"`
	Collect  string    `json:"collectError,omitempty"`
}

// Statuses.
const (
	Passed   = "passed"
	Failed   = "failed"
	Restored = "restored"
	NotRun   = "not-run"
)

// Runner executes phases in order under Evidence.
type Runner struct {
	// Evidence is the directory that receives one subdirectory per phase and
	// phases.json.
	Evidence string
	// Restored skips provisioning phases: their result came from a checkpoint.
	Restored bool
	// OnStart observes a phase beginning, for example to mark a probe
	// timeline. OnEnd observes its record.
	OnStart func(name string, at time.Time)
	OnEnd   func(Record)
	// CollectTimeout bounds each Collect; the default is two minutes.
	CollectTimeout time.Duration
	Now            func() time.Time
}

var phaseName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Run executes the phases and returns every record, including phases that
// did not run because an earlier one failed. The error is the first failure.
func (r Runner) Run(ctx context.Context, phases ...Phase) ([]Record, error) {
	now := r.Now
	if now == nil {
		now = time.Now
	}
	if !filepath.IsAbs(r.Evidence) {
		return nil, fmt.Errorf("an absolute evidence directory is required")
	}
	seen := map[string]bool{}
	for _, p := range phases {
		if !phaseName.MatchString(p.Name) || seen[p.Name] || p.Run == nil {
			return nil, fmt.Errorf("phase %q needs a unique lowercase name and a Run function", p.Name)
		}
		seen[p.Name] = true
	}
	if err := os.MkdirAll(r.Evidence, 0o700); err != nil {
		return nil, err
	}
	records := make([]Record, len(phases))
	for i, p := range phases {
		records[i] = Record{Name: p.Name, Status: NotRun, Evidence: filepath.Join(r.Evidence, fmt.Sprintf("%02d-%s", i+1, p.Name))}
	}
	var failure error
	for i, p := range phases {
		record := &records[i]
		if failure != nil {
			continue
		}
		if err := os.MkdirAll(record.Evidence, 0o700); err != nil {
			return records, err
		}
		record.Started = now().UTC()
		if r.OnStart != nil {
			r.OnStart(p.Name, record.Started)
		}
		if p.Provisioning && r.Restored {
			record.Status = Restored
		} else {
			runCtx, cancel := ctx, context.CancelFunc(func() {})
			if p.Timeout > 0 {
				runCtx, cancel = context.WithTimeout(ctx, p.Timeout)
			}
			err := p.Run(runCtx, record.Evidence)
			cancel()
			record.Status = Passed
			if err != nil {
				record.Status, record.Error = Failed, err.Error()
				failure = fmt.Errorf("phase %s: %w", p.Name, err)
			}
			if p.Collect != nil {
				timeout := r.CollectTimeout
				if timeout == 0 {
					timeout = 2 * time.Minute
				}
				// Evidence of a failure matters most when the run's own
				// context is what expired.
				collectCtx, done := context.WithTimeout(context.WithoutCancel(ctx), timeout)
				if err := p.Collect(collectCtx, record.Evidence); err != nil {
					record.Collect = err.Error()
				}
				done()
			}
		}
		record.Finished = now().UTC()
		record.Seconds = record.Finished.Sub(record.Started).Seconds()
		if r.OnEnd != nil {
			r.OnEnd(*record)
		}
		if err := r.write(records); err != nil && failure == nil {
			failure = err
		}
	}
	if err := r.write(records); err != nil && failure == nil {
		failure = err
	}
	return records, failure
}

// write keeps phases.json current after every phase, so a run that is killed
// still shows how far it got.
func (r Runner) write(records []Record) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(r.Evidence, ".phases.json")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(r.Evidence, "phases.json"))
}
