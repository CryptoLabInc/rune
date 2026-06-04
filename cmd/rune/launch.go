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

	"github.com/CryptoLabInc/rune-cli/internal/bootstrap"
)

const gracefulShutdownGrace = 5 * time.Second

func execInstalledBinary(ctx context.Context, binDir, name string, args []string, extraEnv []string, stderr io.Writer) int {
	binPath := filepath.Join(binDir, name)

	if _, err := os.Stat(binPath); err != nil {
		fmt.Fprintf(stderr,
			"rune: %s not installed at %s (exit 127).\n"+
				"Agent recovery: invoke %s, then re-attempt the failed slash command.\n",
			name, binPath, bootstrap.AgentInstallRecoveryHint())
		return 127 // not installed (missing binary)
	}

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
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
