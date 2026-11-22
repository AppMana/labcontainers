package windows

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Guest is the file and command surface of client.Node used to run a
// deployment's own node scripts in a Windows VM.
type Guest interface {
	Executor
	Put(ctx context.Context, path string, mode uint32, content []byte) error
}

// Kubelet runs a Windows node the way a deployment that does not use a
// distribution's bundled worker does: a kubelet service the deployment's
// PrepareNode script registers, started through its StartKubelet script,
// against a container runtime that is already serving. Both scripts are the
// deployment's own and run as given; this type only supplies the order of
// operations around them and the files a fresh node needs first.
type Kubelet struct {
	// PrepareNode downloads the kubelet for a version and registers the
	// kubelet service. It is invoked with -KubernetesVersion v<Version>
	// -ContainerRuntime ContainerD.
	PrepareNode []byte
	// StartKubelet is the service's entry point, written to
	// C:\k\StartKubelet.ps1.
	StartKubelet []byte
	// Version is the Kubernetes version without a leading v.
	Version string
	// Timeout bounds PrepareNode; the default is ten minutes.
	Timeout time.Duration
}

var kubeletVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func (k Kubelet) validate() error {
	if len(k.PrepareNode) == 0 || len(k.StartKubelet) == 0 || !kubeletVersion.MatchString(k.Version) {
		return fmt.Errorf("both node scripts and an exact Kubernetes version are required")
	}
	return nil
}

func runScript(ctx context.Context, guest Executor, timeout time.Duration, what, script string) (string, error) {
	r, err := guest.ExecWithTimeout(ctx, timeout, PowerShellCommand(script)...)
	if err != nil {
		return "", fmt.Errorf("%s: %w", what, err)
	}
	if r == nil {
		return "", fmt.Errorf("%s: missing guest result", what)
	}
	if r.ExitCode != 0 {
		return string(r.Stdout), fmt.Errorf("%s exited %d: %s %s", what, r.ExitCode, r.Stdout, r.Stderr)
	}
	return string(r.Stdout), nil
}

func (k Kubelet) prepare(ctx context.Context, guest Guest) (string, error) {
	if err := guest.Put(ctx, `C:\PrepareNode.ps1`, 0o600, k.PrepareNode); err != nil {
		return "", err
	}
	timeout := k.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	r, err := guest.ExecWithTimeout(ctx, timeout, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", `C:\PrepareNode.ps1`, "-KubernetesVersion", "v"+k.Version, "-ContainerRuntime", "ContainerD")
	if err != nil {
		return "", fmt.Errorf("PrepareNode: %w", err)
	}
	if r.ExitCode != 0 {
		return string(r.Stdout), fmt.Errorf("PrepareNode exited %d: %s %s", r.ExitCode, r.Stdout, r.Stderr)
	}
	return string(r.Stdout), nil
}

// Join turns a fresh guest into a node. bootstrapKubeconfig is a kubeconfig
// carrying a bootstrap token and the cluster's certificate authority; the
// kubelet replaces it with its own credentials. Join refuses a guest that
// already holds one, so it can never discard a node's rotated identity.
func (k Kubelet) Join(ctx context.Context, guest Guest, bootstrapKubeconfig []byte) (string, error) {
	if err := k.validate(); err != nil {
		return "", err
	}
	if !strings.Contains(string(bootstrapKubeconfig), "certificate-authority-data:") {
		return "", fmt.Errorf("the bootstrap kubeconfig must embed the cluster certificate authority")
	}
	if _, err := runScript(ctx, guest, time.Minute, "fresh node check", `if(Test-Path C:\bootstrap-kubeconfig.yaml){throw 'node already holds a bootstrap kubeconfig'}; if(Get-Service kubelet -ErrorAction SilentlyContinue){throw 'node already has a kubelet service'}; New-Item -ItemType Directory -Force C:\k | Out-Null`); err != nil {
		return "", err
	}
	for _, path := range []string{`C:\bootstrap-kubeconfig.yaml`, `C:\k\config`} {
		if err := guest.Put(ctx, path, 0o600, bootstrapKubeconfig); err != nil {
			return "", err
		}
	}
	out, err := k.prepare(ctx, guest)
	if err != nil {
		return out, err
	}
	// The kubelet verifies its clients against the cluster authority before
	// it has any credentials of its own.
	if _, err := runScript(ctx, guest, time.Minute, "kubelet support files", `New-Item -ItemType Directory -Force C:\var\lib\kubelet\pki | Out-Null
foreach($f in 'C:\var\lib\kubelet\kubeadm-flags.env','C:\var\lib\kubelet\config.yaml'){ if(-not (Test-Path $f)){ New-Item -ItemType File $f | Out-Null } }
$m=[regex]::Match((Get-Content C:\k\config -Raw),'(?ms)certificate-authority-data:\s*(\S+)')
if(-not $m.Success){throw 'no certificate authority in kubeconfig'}
[IO.File]::WriteAllText('C:\var\lib\kubelet\pki\ca.crt',[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($m.Groups[1].Value)),[Text.UTF8Encoding]::new($false))`); err != nil {
		return out, err
	}
	return out, k.start(ctx, guest)
}

// Upgrade moves a joined node's kubelet to Version in place: stop the
// service, let PrepareNode replace the binary and refresh the service, write
// the current StartKubelet, start. Workloads are not drained; their
// containers keep running under the runtime while the kubelet is down.
func (k Kubelet) Upgrade(ctx context.Context, guest Guest) (string, error) {
	if err := k.validate(); err != nil {
		return "", err
	}
	if _, err := runScript(ctx, guest, 2*time.Minute, "stop kubelet", `if(-not (Test-Path C:\bootstrap-kubeconfig.yaml)){throw 'node was never joined'}; Stop-Service kubelet; (Get-Service kubelet).WaitForStatus('Stopped',[TimeSpan]::FromSeconds(60)); $deadline=[DateTime]::UtcNow.AddSeconds(30); while(@(Get-CimInstance Win32_Process -Filter "Name='kubelet.exe'").Count -ne 0){ if([DateTime]::UtcNow -ge $deadline){throw 'kubelet.exe outlived its service'}; Start-Sleep -Milliseconds 250 }`); err != nil {
		return "", err
	}
	out, err := k.prepare(ctx, guest)
	if err != nil {
		return out, err
	}
	return out, k.start(ctx, guest)
}

func (k Kubelet) start(ctx context.Context, guest Guest) error {
	if err := guest.Put(ctx, `C:\k\StartKubelet.ps1`, 0o600, k.StartKubelet); err != nil {
		return err
	}
	_, err := runScript(ctx, guest, 2*time.Minute, "start kubelet", `Start-Service kubelet; (Get-Service kubelet).WaitForStatus('Running',[TimeSpan]::FromSeconds(60))`)
	if err != nil {
		return err
	}
	installed, err := InstalledKubelet(ctx, guest)
	if err != nil {
		return err
	}
	if installed != k.Version {
		return fmt.Errorf("the node's kubelet is %s after preparing %s", installed, k.Version)
	}
	return nil
}

// InstalledKubelet reads the version of the kubelet binary the service runs.
func InstalledKubelet(ctx context.Context, guest Executor) (string, error) {
	out, err := runScript(ctx, guest, time.Minute, "kubelet version", `$v=& C:\k\kubelet.exe --version; if($LASTEXITCODE -ne 0){throw 'kubelet --version failed'}; $v`)
	if err != nil {
		return "", err
	}
	match := regexp.MustCompile(`v(\d+\.\d+\.\d+)`).FindStringSubmatch(out)
	if match == nil {
		return "", fmt.Errorf("unrecognized kubelet version %q", strings.TrimSpace(out))
	}
	return match[1], nil
}

// RuntimeTransaction drives a deployment's side-by-side container runtime
// replacement script. The script is the deployment's own; it must accept
// -Mode Stage|Apply, -Archive, -ArchiveSHA256 and -Version, print a JSON
// object with a "stage" directory when staging, and print CHANGED or
// UNCHANGED when applying. Draining the node and stopping its kubelet around
// Apply remain the caller's procedure.
type RuntimeTransaction struct {
	Script []byte
	// Path is where the script is written on the guest.
	Path string
}

var runtimeVersion = regexp.MustCompile(`^\d+\.\d+\.\d+[-.a-zA-Z0-9]*$`)

func (t RuntimeTransaction) run(ctx context.Context, guest Guest, timeout time.Duration, args ...string) (string, error) {
	if len(t.Script) == 0 || !strings.HasPrefix(strings.ToUpper(t.Path), `C:\`) {
		return "", fmt.Errorf("the transaction script and its absolute guest path are required")
	}
	if err := guest.Put(ctx, t.Path, 0o600, t.Script); err != nil {
		return "", err
	}
	argv := append([]string{"powershell.exe", "-NoProfile", "-NonInteractive", "-File", t.Path}, args...)
	r, err := guest.ExecWithTimeout(ctx, timeout, argv...)
	if err != nil {
		return "", fmt.Errorf("runtime transaction: %w", err)
	}
	if r.ExitCode != 0 {
		return string(r.Stdout), fmt.Errorf("runtime transaction %v exited %d: %s %s", args, r.ExitCode, r.Stdout, r.Stderr)
	}
	return string(r.Stdout), nil
}

func validRuntime(version, sha256 string) error {
	if !runtimeVersion.MatchString(version) || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(sha256) {
		return fmt.Errorf("an exact runtime version and archive SHA-256 are required")
	}
	return nil
}

// Stage unpacks and verifies the archive beside the serving runtime without
// touching it, and returns the staged release directory.
func (t RuntimeTransaction) Stage(ctx context.Context, guest Guest, archive, sha256, version string) (string, error) {
	if err := validRuntime(version, sha256); err != nil {
		return "", err
	}
	out, err := t.run(ctx, guest, 5*time.Minute, "-Mode", "Stage", "-Archive", archive, "-ArchiveSHA256", sha256, "-Version", version)
	if err != nil {
		return "", err
	}
	var staged struct {
		Stage string `json:"stage"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &staged); err != nil || staged.Stage == "" {
		return "", fmt.Errorf("runtime transaction returned no stage: %q", out)
	}
	return staged.Stage, nil
}

// Apply switches the runtime service to the staged release and reports
// whether anything changed. extra is passed through for switches a fresh
// installation needs.
func (t RuntimeTransaction) Apply(ctx context.Context, guest Guest, sha256, version string, extra ...string) (bool, error) {
	if err := validRuntime(version, sha256); err != nil {
		return false, err
	}
	out, err := t.run(ctx, guest, 5*time.Minute, append([]string{"-Mode", "Apply", "-ArchiveSHA256", sha256, "-Version", version}, extra...)...)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		switch strings.TrimSpace(line) {
		case "CHANGED":
			return true, nil
		case "UNCHANGED":
			return false, nil
		}
	}
	return false, fmt.Errorf("runtime transaction reported neither CHANGED nor UNCHANGED: %q", out)
}
