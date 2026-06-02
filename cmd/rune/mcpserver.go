package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/CryptoLabInc/rune-cli/internal/bootstrap"
)

func runMCPServer(ctx context.Context, args []string, stderr io.Writer) int {
	paths, err := bootstrap.Resolve()
	if err != nil {
		fmt.Fprintf(stderr, "rune: cannot resolve home directories: %v\n", err)
		return 1
	}

	// Fresh `claude plugin install rune` try to spawn MCP server via
	// "${CLAUDE_PLUGIN_ROOT}/bin/rune mcp-server" which does not exist yet.
	// Self-install rune-mcp itself in this case.
	if _, statErr := os.Stat(paths.RuneMCPBinary); statErr != nil {
		fmt.Fprintln(stderr, "rune: rune-mcp not installed yet; fetching before launch...")
		manifest := manifestURL
		if env := os.Getenv("RUNE_MANIFEST"); env != "" {
			manifest = env
		}

		// Concurrency-safe install
		_, instErr := bootstrap.Install(ctx, bootstrap.InstallOptions{
			ManifestURL: manifest,
			Target:      []string{bootstrap.StepRuneMCP},
			Log: func(format string, a ...any) {
				fmt.Fprintf(stderr, format+"\n", a...)
			},
		})
		if instErr != nil {
			// Sessions which do not invoke install wait here
			if !waitForFile(ctx, paths.RuneMCPBinary, bootstrap.InstallLockTimeout) {
				fmt.Fprintf(stderr, "rune: failed to install rune-mcp: %v\n", instErr)
				return 1
			}

			fmt.Fprintln(stderr, "rune: rune-mcp installed by a concurrent session")
		}
	}

	return execInstalledBinary(ctx, paths.RuneBin, "rune-mcp", args, nil, stderr)
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
