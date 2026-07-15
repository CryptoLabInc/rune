package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/CryptoLabInc/rune-cli/internal/bootstrap"
)

const mcpSelfhealBudget = 25 * time.Second // (download:25s + (exec + MCP handshake):5s) < plugin manifest 30s MCP connection timeout

func runMCPServer(ctx context.Context, args []string, stderr io.Writer) int {
	// Dev override: point the plugin at a locally-built rune-mcp (e.g. a feature
	// branch that the release pin does not yet ship). When RUNE_MCP_BIN is set we
	// exec that binary directly and skip install + auto-update entirely, so the
	// pinned release is never fetched over it.
	if dev := os.Getenv("RUNE_MCP_BIN"); dev != "" {
		if _, statErr := os.Stat(dev); statErr != nil {
			fmt.Fprintf(stderr, "rune: RUNE_MCP_BIN=%s not usable: %v\n", dev, statErr)
			return 127
		}
		fmt.Fprintf(stderr, "rune: using dev rune-mcp override %s (install + auto-update skipped)\n", dev)
		return execInstalledBinary(ctx, filepath.Dir(dev), filepath.Base(dev), args, nil, stderr)
	}

	paths, err := bootstrap.Resolve()
	if err != nil {
		fmt.Fprintf(stderr, "rune: cannot resolve home directories: %v\n", err)
		return 1
	}

	// Fresh `claude plugin install rune` try to spawn MCP server via
	// "${CLAUDE_PLUGIN_ROOT}/bin/rune mcp-server" which does not exist yet.
	// Self-install rune-mcp itself in this case.
	justInstalled := false
	if _, statErr := os.Stat(paths.RuneMCPBinary); statErr != nil {
		fmt.Fprintln(stderr, "rune: rune-mcp not installed yet; fetching before launch...")
		manifest := manifestURL
		if env := os.Getenv("RUNE_MANIFEST"); env != "" {
			manifest = env
		}

		healCtx, cancel := context.WithTimeout(ctx, mcpSelfhealBudget)
		defer cancel()

		// Concurrency-safe install
		_, instErr := bootstrap.Install(healCtx, bootstrap.InstallOptions{
			ManifestURL: manifest,
			Target:      []string{bootstrap.StepRuneMCP},
			Log: func(format string, a ...any) {
				fmt.Fprintf(stderr, format+"\n", a...)
			},
		})
		if instErr != nil {
			// If error is not ErrInstallInProgress, it is unavoidable error
			if !errors.Is(instErr, bootstrap.ErrInstallInProgress) {
				fmt.Fprintf(stderr, "rune: cannot install rune-mcp: %v\n", instErr)
				return 1
			}

			// Another session hold lock
			if !waitForFile(healCtx, paths.RuneMCPBinary, mcpSelfhealBudget) {
				fmt.Fprintf(stderr, "rune: rune-mcp install still in progress after %s: %v\n", mcpSelfhealBudget, instErr)
				fmt.Fprintln(stderr, "     another session is installing it; reconnect via /mcp once it completes")
				return 1
			}

			fmt.Fprintln(stderr, "rune: rune-mcp installed by a concurrent session")
		}

		justInstalled = true
	}

	if !justInstalled {
		tryAutoCheck(paths, stderr)
	}

	return execInstalledBinary(ctx, paths.RuneBin, "rune-mcp", args, nil, stderr)
}

var spawnUpdateFn = spawnDetachedUpdate // background update launcher

var errBackgroundUnsupported = errors.New("detached background update not supported on this platform")

func resolvedManifest() string {
	m := manifestURL
	if env := os.Getenv("RUNE_MANIFEST"); env != "" {
		m = env
	}
	return m
}

func tryAutoCheck(paths *bootstrap.Paths, stderr io.Writer) {
	if bootstrap.AutoUpdateDisabled() {
		return
	}

	manifest := resolvedManifest()
	if manifest == "" {
		return
	}
	if !bootstrap.ShouldAutoCheck(paths.AutoCheckStamp, bootstrap.AutoCheckInterval(), time.Now()) {
		return
	}

	// Record first to prevent re-spawn on reconnection
	if err := bootstrap.RecordAutoCheck(paths.AutoCheckStamp, time.Now()); err != nil {
		return
	}
	if err := spawnUpdateFn(paths, manifest); err != nil {
		fmt.Fprintf(stderr, "rune: background update check not started: %v\n", err)
	}
}

func spawnDetachedUpdate(paths *bootstrap.Paths, manifest string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(paths.UpdateLog), 0o700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(paths.UpdateLog, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()

	// Runed is excluded since mcp server does not handle its lifecycle
	cmd := exec.Command(exe, "update", "--only", bootstrap.StepRuneMCP, "--manifest-url", manifest)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if !setDetached(cmd) {
		return errBackgroundUnsupported
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() { _ = cmd.Wait() }() // child exit

	return nil
}

func waitForFile(ctx context.Context, path string, timeout time.Duration) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-tick.C:
			if _, err := os.Stat(path); err == nil {
				return true
			}
		}
	}
}
