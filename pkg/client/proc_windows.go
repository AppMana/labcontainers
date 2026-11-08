//go:build windows

package client

import "os/exec"

func configureDaemonProcess(*exec.Cmd) {}
