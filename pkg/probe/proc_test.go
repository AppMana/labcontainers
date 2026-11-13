package probe

import (
	"os/exec"
	"syscall"
)

func processGroup() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }

func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
	}
}
