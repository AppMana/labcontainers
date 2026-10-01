// Package windows contains guest provisioning operations shared by downstream
// labs. It does not select images, add networks, or install a container runtime.
package windows

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	labv1 "github.com/appmana/labcontainers/api/v1"
)

// Executor is implemented by client.Node. Keeping execution on that boundary
// preserves serial control and native per-command cancellation/deadlines.
type Executor interface {
	ExecWithTimeout(context.Context, time.Duration, ...string) (*labv1.ExecResponse, error)
}

type FeatureOptions struct {
	Names                  []string
	IncludeManagementTools bool
	AllowReboot            bool
	InstallTimeout         time.Duration
	RebootTimeout          time.Duration
	PollInterval           time.Duration
}

// PowerShellCommand keeps script text out of the native command-line parser.
// Native executables invoked inside the script still need literal argv arrays.
func PowerShellCommand(script string) []string {
	words := utf16.Encode([]rune("$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'; " + script))
	body := make([]byte, len(words)*2)
	for i, w := range words {
		binary.LittleEndian.PutUint16(body[i*2:], w)
	}
	return []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(body)}
}

type featureState struct {
	Installed      bool   `json:"installed"`
	RebootRequired bool   `json:"rebootRequired"`
	BootID         string `json:"bootId"`
}

// EnsureFeatures installs only explicitly named Windows Server features. A
// requested reboot is a guest reboot, not a crash/replacement. Success after a
// reboot requires both a different boot identity and all requested features.
func EnsureFeatures(ctx context.Context, guest Executor, options FeatureOptions) error {
	if guest == nil || len(options.Names) == 0 {
		return fmt.Errorf("guest and explicit feature names required")
	}
	valid := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)
	names := make([]string, len(options.Names))
	for i, name := range options.Names {
		if !valid.MatchString(name) {
			return fmt.Errorf("invalid Windows feature %q", name)
		}
		names[i] = "'" + name + "'"
	}
	if options.InstallTimeout == 0 {
		options.InstallTimeout = 8 * time.Minute
	}
	if options.RebootTimeout == 0 {
		options.RebootTimeout = 5 * time.Minute
	}
	if options.PollInterval == 0 {
		options.PollInterval = 2 * time.Second
	}
	if options.InstallTimeout < time.Millisecond || options.RebootTimeout < time.Millisecond || options.PollInterval < time.Millisecond {
		return fmt.Errorf("positive provisioning deadlines required")
	}
	query := `$names=@(` + strings.Join(names, ",") + `); $features=@(Get-WindowsFeature -Name $names); if($features.Count -ne $names.Count){throw 'Requested feature not found'}; $missing=@($features | Where-Object {-not $_.Installed}); `
	install := query + `$reboot=$false; if($missing.Count){$result=Install-WindowsFeature -Name $missing.Name`
	if options.IncludeManagementTools {
		install += " -IncludeManagementTools"
	}
	install += `; if(-not $result.Success){throw 'Windows feature installation failed'}; $reboot=([string]$result.RestartNeeded -ne 'No')}; ` + query + `@{installed=($missing.Count -eq 0);rebootRequired=$reboot;bootId=[string](Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToUniversalTime().Ticks} | ConvertTo-Json -Compress`
	execute := func(ctx context.Context, timeout time.Duration, script string) ([]byte, error) {
		r, err := guest.ExecWithTimeout(ctx, timeout, PowerShellCommand(script)...)
		if err != nil {
			return nil, err
		}
		if r == nil {
			return nil, fmt.Errorf("missing guest result")
		}
		if r.ExitCode != 0 {
			return nil, fmt.Errorf("Windows provisioning exited %d: %s %s", r.ExitCode, r.Stdout, r.Stderr)
		}
		return r.Stdout, nil
	}
	decode := func(body []byte) (featureState, error) {
		var s featureState
		if err := json.Unmarshal(body, &s); err != nil {
			return s, fmt.Errorf("invalid Windows feature result: %w", err)
		}
		if s.BootID == "" {
			return s, fmt.Errorf("missing Windows boot identity")
		}
		return s, nil
	}
	body, err := execute(ctx, options.InstallTimeout, install)
	if err != nil {
		return err
	}
	before, err := decode(body)
	if err != nil {
		return err
	}
	if !before.RebootRequired {
		if !before.Installed {
			return fmt.Errorf("requested Windows features remain uninstalled")
		}
		return nil
	}
	if !options.AllowReboot {
		return fmt.Errorf("Windows feature installation requires an explicitly allowed reboot")
	}
	if _, err := execute(ctx, 15*time.Second, `shutdown.exe /r /t 2; if($LASTEXITCODE -ne 0){exit $LASTEXITCODE}`); err != nil {
		return err
	}
	rebootCtx, cancel := context.WithTimeout(ctx, options.RebootTimeout)
	defer cancel()
	probe := query + `@{installed=($missing.Count -eq 0);bootId=[string](Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToUniversalTime().Ticks} | ConvertTo-Json -Compress`
	var last error
	for {
		body, err := execute(rebootCtx, 15*time.Second, probe)
		if err == nil {
			var after featureState
			after, err = decode(body)
			if err == nil && after.Installed && after.BootID != before.BootID {
				return nil
			}
			if err == nil {
				err = fmt.Errorf("waiting for changed boot identity and installed features")
			}
		}
		last = err
		select {
		case <-rebootCtx.Done():
			return fmt.Errorf("Windows feature reboot: %w (last observation: %v)", rebootCtx.Err(), last)
		case <-time.After(options.PollInterval):
		}
	}
}
