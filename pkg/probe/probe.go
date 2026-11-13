// Package probe measures reachability continuously from inside a lab and
// accounts for every interruption. A loop generated here runs in a guest or a
// pod, independent of the control channel, and appends one JSON line per
// attempt. Outages turns those lines into per-path outage windows, so a
// procedure that is allowed to interrupt traffic is reported by how long it
// did, not by whether a check happened to run at a quiet moment.
package probe

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Sample is one attempt on one path.
type Sample struct {
	Time   time.Time `json:"t"`
	Source string    `json:"from"`
	Path   string    `json:"path"`
	OK     bool      `json:"ok"`
	Detail string    `json:"detail,omitempty"`
}

// Target is one path a loop measures. Exactly one of URL and DNSName is set.
type Target struct {
	// Name identifies the path in samples and reports.
	Name string
	// URL is fetched over HTTP(S); any response with Status counts, or any
	// response at all when Status is 0. Certificates are not verified: the
	// probe measures reachability.
	URL    string
	Status int
	// DNSName is resolved as an A record through DNSServer.
	DNSName   string
	DNSServer string
}

var safeWord = regexp.MustCompile(`^[A-Za-z0-9._:/\[\]?&=%+@-]+$`)

func (t Target) validate() error {
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`).MatchString(t.Name) {
		return fmt.Errorf("probe target name %q must be lowercase letters, digits and hyphens", t.Name)
	}
	switch {
	case t.URL != "" && t.DNSName == "" && t.DNSServer == "":
		if !safeWord.MatchString(t.URL) || !(strings.HasPrefix(t.URL, "http://") || strings.HasPrefix(t.URL, "https://")) {
			return fmt.Errorf("probe target %s: unsupported URL %q", t.Name, t.URL)
		}
	case t.URL == "" && t.DNSName != "" && t.DNSServer != "":
		if !safeWord.MatchString(t.DNSName) || !safeWord.MatchString(t.DNSServer) {
			return fmt.Errorf("probe target %s: unsupported DNS name or server", t.Name)
		}
	default:
		return fmt.Errorf("probe target %s needs either a URL or a DNS name with its server", t.Name)
	}
	return nil
}

// Loop describes the loops one source runs: one per target, each appending to
// its own file Directory/<source>-<target>.jsonl so a slow or failing path
// never delays the others.
type Loop struct {
	Source    string
	Directory string
	Interval  time.Duration
	// Timeout bounds one attempt; an attempt that exceeds it is a failure.
	Timeout time.Duration
	Targets []Target
}

func (l Loop) validate() error {
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`).MatchString(l.Source) || l.Directory == "" || strings.ContainsAny(l.Directory, "'\"`$ \n") {
		return fmt.Errorf("probe loop needs a simple source name and a directory without quoting characters")
	}
	if l.Interval < time.Second || l.Timeout < time.Second || len(l.Targets) == 0 {
		return fmt.Errorf("probe loop needs an interval and timeout of at least one second and a target")
	}
	seen := map[string]bool{}
	for _, t := range l.Targets {
		if err := t.validate(); err != nil {
			return err
		}
		if seen[t.Name] {
			return fmt.Errorf("probe target %s is declared twice", t.Name)
		}
		seen[t.Name] = true
	}
	return nil
}

// File is where the loop for target writes.
func (l Loop) File(target string) string {
	sep := "/"
	if strings.Contains(l.Directory, `\`) {
		sep = `\`
	}
	return strings.TrimRight(l.Directory, `/\`) + sep + l.Source + "-" + target + ".jsonl"
}

// Shell is a POSIX sh program needing curl and, for DNS targets, dig. It
// runs until killed and appends across restarts.
func (l Loop) Shell() (string, error) {
	if err := l.validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "mkdir -p '%s'\n", l.Directory)
	fmt.Fprintf(&b, "emit() { printf '{\"t\":\"%%s\",\"from\":\"%s\",\"path\":\"%%s\",\"ok\":%%s,\"detail\":\"%%s\"}\\n' \"$1\" \"$2\" \"$3\" \"$4\" >> \"$5\"; }\n", l.Source)
	seconds, timeout := int(l.Interval/time.Second), int(l.Timeout/time.Second)
	for _, t := range l.Targets {
		var attempt string
		if t.URL != "" {
			want := `[ "$code" != 000 ]`
			if t.Status != 0 {
				want = fmt.Sprintf(`[ "$code" = %d ]`, t.Status)
			}
			attempt = fmt.Sprintf(`code=$(curl -ks -o /dev/null -w '%%{http_code}' --connect-timeout %d --max-time %d '%s'); rc=$?; if %s; then ok=true; else ok=false; fi; detail="http=$code curl=$rc"`, timeout, timeout, t.URL, want)
		} else {
			attempt = fmt.Sprintf(`answer=$(dig +time=%d +tries=1 +short '@%s' '%s' A 2>/dev/null | head -n1); if printf '%%s' "$answer" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$'; then ok=true; else ok=false; fi; detail="answer=$answer"`, timeout, t.DNSServer, t.DNSName)
		}
		fmt.Fprintf(&b, "( while :; do t=$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ); %s; emit \"$t\" '%s' \"$ok\" \"$detail\" '%s'; sleep %d; done ) &\n", attempt, t.Name, l.File(t.Name), seconds)
	}
	b.WriteString("wait\n")
	return b.String(), nil
}

// PowerShell is the same loop for Windows PowerShell 5.1, needing curl.exe
// and, for DNS targets, nslookup.exe.
func (l Loop) PowerShell() (string, error) {
	if err := l.validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	seconds, timeout := int(l.Interval/time.Second), int(l.Timeout/time.Second)
	fmt.Fprintf(&b, "$ErrorActionPreference='Continue'\nNew-Item -ItemType Directory -Force -Path '%s' | Out-Null\n", l.Directory)
	b.WriteString("$loop={param($name,$kind,$a,$b,$file,$source,$interval,$timeout)\n" +
		" while($true){\n" +
		"  $t=[DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ'); $ok=$false; $detail=''\n" +
		"  try{\n" +
		"   if($kind -eq 'http'){\n" +
		"    $code=(& curl.exe -ks -o NUL -w '%{http_code}' --connect-timeout $timeout --max-time $timeout $a) -join ''; $rc=$LASTEXITCODE\n" +
		"    if($b -eq '0'){$ok=($code -ne '000' -and $code -ne '')}else{$ok=($code -eq $b)}\n" +
		"    $detail=\"http=$code curl=$rc\"\n" +
		"   }else{\n" +
		"    $out=(& nslookup.exe \"-timeout=$timeout\" -retry=1 -type=A $a $b 2>&1 | Out-String)\n" +
		"    $m=[regex]::Match($out,'(?s)Name:.*?Address(?:es)?:\\s*([0-9]+\\.[0-9]+\\.[0-9]+\\.[0-9]+)')\n" +
		"    $ok=$m.Success; $detail='answer='+$m.Groups[1].Value\n" +
		"   }\n" +
		"  }catch{$detail='error'}\n" +
		"  $line='{\"t\":\"'+$t+'\",\"from\":\"'+$source+'\",\"path\":\"'+$name+'\",\"ok\":'+$ok.ToString().ToLowerInvariant()+',\"detail\":\"'+$detail+'\"}'\n" +
		"  [IO.File]::AppendAllText($file,$line+\"`n\",[Text.UTF8Encoding]::new($false))\n" +
		"  Start-Sleep -Seconds $interval\n" +
		" }\n}\n$jobs=@()\n")
	for _, t := range l.Targets {
		kind, a, bArg := "http", t.URL, fmt.Sprint(t.Status)
		if t.DNSName != "" {
			kind, a, bArg = "dns", t.DNSName, t.DNSServer
		}
		fmt.Fprintf(&b, "$jobs+=Start-Job -ScriptBlock $loop -ArgumentList '%s','%s','%s','%s','%s','%s',%d,%d\n", t.Name, kind, a, bArg, l.File(t.Name), l.Source, seconds, timeout)
	}
	// Keep the container's main process alive for as long as the loops run,
	// and surface a loop that died instead of going silent.
	b.WriteString("while($true){ foreach($j in $jobs){ if($j.State -ne 'Running'){ Receive-Job $j; throw \"probe loop $($j.Id) stopped: $($j.State)\" } }; Start-Sleep -Seconds 30 }\n")
	return b.String(), nil
}

// Parse reads sample lines. A final line cut short by a concurrent append or
// a crash is skipped; any other malformed line is an error.
func Parse(r io.Reader) ([]Sample, error) {
	var samples []Sample
	reader := bufio.NewReaderSize(r, 1<<16)
	for number := 1; ; number++ {
		line, err := reader.ReadBytes('\n')
		complete := err == nil
		if err != nil && err != io.EOF {
			return nil, err
		}
		line = bytes.TrimSpace(bytes.TrimPrefix(line, []byte("\xef\xbb\xbf")))
		if len(line) != 0 {
			var s Sample
			if jsonErr := json.Unmarshal(line, &s); jsonErr != nil || s.Time.IsZero() || s.Source == "" || s.Path == "" {
				if complete {
					return nil, fmt.Errorf("probe sample line %d is malformed: %q", number, line)
				}
			} else {
				samples = append(samples, s)
			}
		}
		if !complete {
			return samples, nil
		}
	}
}

// Mark names the phase that begins at Time. Phases explain outages; they do
// not excuse them.
type Mark struct {
	Time  time.Time `json:"time"`
	Phase string    `json:"phase"`
}

// Window is a run of consecutive failures on one path.
type Window struct {
	Source string `json:"from"`
	Path   string `json:"path"`
	// LastOK is the last success before the outage; zero if none was seen.
	LastOK time.Time `json:"lastOk"`
	// Start is the first failed attempt and End the first success after it.
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	// Open means the samples end while the path is still failing; End is
	// then the last failed attempt.
	Open     bool          `json:"open,omitempty"`
	Failures int           `json:"failures"`
	Duration time.Duration `json:"durationNanos"`
	// AtMost bounds the outage from the last success to the first success.
	AtMost time.Duration `json:"atMostNanos"`
	Detail string        `json:"detail,omitempty"`
	Phases []string      `json:"phases,omitempty"`
}

// Gap is a stretch with no attempts on a path: the loop itself was not
// running (its node was down, its pod was being replaced). Reachability in a
// gap is unknown, which is not the same as available.
type Gap struct {
	Source   string        `json:"from"`
	Path     string        `json:"path"`
	Start    time.Time     `json:"start"`
	End      time.Time     `json:"end"`
	Duration time.Duration `json:"durationNanos"`
	Phases   []string      `json:"phases,omitempty"`
}

// PathReport accounts for one source and path.
type PathReport struct {
	Source   string        `json:"from"`
	Path     string        `json:"path"`
	Samples  int           `json:"samples"`
	Failures int           `json:"failures"`
	First    time.Time     `json:"first"`
	Last     time.Time     `json:"last"`
	Outages  []Window      `json:"outages"`
	Gaps     []Gap         `json:"gaps"`
	Down     time.Duration `json:"downNanos"`
	Unseen   time.Duration `json:"unseenNanos"`
}

type Report struct {
	Marks []Mark       `json:"marks,omitempty"`
	Paths []PathReport `json:"paths"`
}

// Outages accounts for every path in samples. Consecutive attempts on a path
// further apart than gapAfter are reported as a gap.
func Outages(samples []Sample, marks []Mark, gapAfter time.Duration) Report {
	marks = append([]Mark(nil), marks...)
	sort.SliceStable(marks, func(i, j int) bool { return marks[i].Time.Before(marks[j].Time) })
	type key struct{ source, path string }
	grouped := map[key][]Sample{}
	for _, s := range samples {
		k := key{s.Source, s.Path}
		grouped[k] = append(grouped[k], s)
	}
	keys := make([]key, 0, len(grouped))
	for k := range grouped {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].source != keys[j].source {
			return keys[i].source < keys[j].source
		}
		return keys[i].path < keys[j].path
	})
	report := Report{Marks: marks}
	for _, k := range keys {
		list := grouped[k]
		sort.SliceStable(list, func(i, j int) bool { return list[i].Time.Before(list[j].Time) })
		p := PathReport{Source: k.source, Path: k.path, Samples: len(list), First: list[0].Time, Last: list[len(list)-1].Time, Outages: []Window{}, Gaps: []Gap{}}
		var open *Window
		var lastOK time.Time
		closeWindow := func(end time.Time, stillOpen bool) {
			open.End, open.Open = end, stillOpen
			open.Duration = end.Sub(open.Start)
			if !open.LastOK.IsZero() && !stillOpen {
				open.AtMost = end.Sub(open.LastOK)
			}
			open.Phases = phasesBetween(marks, open.Start, end)
			p.Down += open.Duration
			p.Outages = append(p.Outages, *open)
			open = nil
		}
		for i, s := range list {
			if i > 0 && gapAfter > 0 && s.Time.Sub(list[i-1].Time) > gapAfter {
				gap := Gap{Source: k.source, Path: k.path, Start: list[i-1].Time, End: s.Time, Duration: s.Time.Sub(list[i-1].Time)}
				gap.Phases = phasesBetween(marks, gap.Start, gap.End)
				p.Gaps = append(p.Gaps, gap)
				p.Unseen += gap.Duration
			}
			if s.OK {
				if open != nil {
					closeWindow(s.Time, false)
				}
				lastOK = s.Time
				continue
			}
			p.Failures++
			if open == nil {
				open = &Window{Source: k.source, Path: k.path, Start: s.Time, LastOK: lastOK, Detail: s.Detail}
			}
			open.Failures++
		}
		if open != nil {
			closeWindow(list[len(list)-1].Time, true)
		}
		report.Paths = append(report.Paths, p)
	}
	return report
}

// phasesBetween names every phase in effect at some point of [start, end].
func phasesBetween(marks []Mark, start, end time.Time) []string {
	var phases []string
	for i, mark := range marks {
		next := end.Add(time.Nanosecond)
		if i+1 < len(marks) {
			next = marks[i+1].Time
		}
		if !mark.Time.After(end) && next.After(start) {
			phases = append(phases, mark.Phase)
		}
	}
	return phases
}

// Text renders the report for a log: one line per path, then one per outage
// and gap.
func (r Report) Text() string {
	var b strings.Builder
	for _, p := range r.Paths {
		fmt.Fprintf(&b, "%s -> %s: %d samples, %d failed, %d outages (down %s), %d gaps (unseen %s)\n", p.Source, p.Path, p.Samples, p.Failures, len(p.Outages), p.Down.Round(time.Second), len(p.Gaps), p.Unseen.Round(time.Second))
		for _, w := range p.Outages {
			state := ""
			if w.Open {
				state = " STILL DOWN"
			}
			fmt.Fprintf(&b, "  outage %s .. %s  %s (at most %s)%s  phases=%s  %s\n", w.Start.Format(time.RFC3339), w.End.Format(time.RFC3339), w.Duration.Round(time.Second), w.AtMost.Round(time.Second), state, strings.Join(w.Phases, ","), w.Detail)
		}
		for _, g := range p.Gaps {
			fmt.Fprintf(&b, "  gap    %s .. %s  %s  phases=%s\n", g.Start.Format(time.RFC3339), g.End.Format(time.RFC3339), g.Duration.Round(time.Second), strings.Join(g.Phases, ","))
		}
	}
	return b.String()
}

// Within returns the outages and gaps of every path that overlap [start, end].
func (r Report) Within(start, end time.Time) (outages []Window, gaps []Gap) {
	for _, p := range r.Paths {
		for _, w := range p.Outages {
			if !w.Start.After(end) && !w.End.Before(start) {
				outages = append(outages, w)
			}
		}
		for _, g := range p.Gaps {
			if g.Start.Before(end) && g.End.After(start) {
				gaps = append(gaps, g)
			}
		}
	}
	return outages, gaps
}
