package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const gracefulShutdownGrace = 5 * time.Second

func execInstalledBinary(ctx context.Context, binDir, name string, args []string, stderr io.Writer) int {
	binPath := filepath.Join(binDir, name)

	if _, err := os.Stat(binPath); err != nil {
		fmt.Fprintf(stderr,
			"rune: %s not installed at %s.\nRun `rune install` first (then restart your Claude session if you were trying to launch rune-mcp).\n",
			name, binPath)
		return 127 // not installed (missing binary)
	}

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Forward SIGTERM to the child instead of SIGKILL
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = gracefulShutdownGrace

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(stderr, "rune: launching %s failed: %v\n", name, err)
		return 1
	}

	return 0
}
