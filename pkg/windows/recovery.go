package windows

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"
)

//go:embed unattended_recovery.ps1
var unattendedRecoveryScript string

// ConfigureUnattendedRecovery explicitly applies the shared image-build boot
// policy to a disposable lab guest made from an older image. It avoids an
// interactive WinRE prompt following crash injection. It neither repairs data
// nor changes driver-signing policy. Real boot/readback assertions still apply.
func ConfigureUnattendedRecovery(ctx context.Context, guest Executor) error {
	r, err := guest.ExecWithTimeout(ctx, time.Minute, PowerShellCommand(unattendedRecoveryScript)...)
	if err != nil {
		return err
	}
	if r == nil || r.ExitCode != 0 || !strings.Contains(string(r.Stdout), "UNATTENDED_RECOVERY_CONFIGURED") {
		return fmt.Errorf("unattended Windows recovery policy not verified: %+v", r)
	}
	return nil
}
