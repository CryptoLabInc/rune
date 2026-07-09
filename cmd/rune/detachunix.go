//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

func setDetached(cmd *exec.Cmd) bool {
	// Make child as own session
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true

	return true
}
