//go:build unix

package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func applyDetachAttrs(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}

func redirectStdio(logPath string) error {
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer logFile.Close()

	devNull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	defer devNull.Close()

	// Replace stdin with /dev/null: prevent user interaction
	if err := unix.Dup2(int(devNull.Fd()), int(os.Stdin.Fd())); err != nil {
		return fmt.Errorf("dup2 stdin: %w", err)
	}
	// Replace stdout and stderr with log file descriptor: log outputs
	if err := unix.Dup2(int(logFile.Fd()), int(os.Stdout.Fd())); err != nil {
		return fmt.Errorf("dup2 stdout: %w", err)
	}
	if err := unix.Dup2(int(logFile.Fd()), int(os.Stderr.Fd())); err != nil {
		return fmt.Errorf("dup2 stderr: %w", err)
	}

	return nil
}
