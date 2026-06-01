package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/CryptoLabInc/rune-cli/internal/bootstrap"
	"github.com/CryptoLabInc/rune-cli/internal/supervisor"
)

// runRuned dispatches `rune runed [args...]`:
//   - Without `--detach`: forwards stdio + args to ~/.runed/bin/runed.
//     Runed is executed in the foreground.
//   - With `--detach`: supervisor mode - re-exec itself with setsid()
//     to become a process group leader, redirect stdio to ~/.runed/logs/daemon.log,
//     take ~/.runed/supervisor.lock to prevent race, and watch runed in a restart loop.
//     The user-facing invocation returns immediately once supervisor is launched.
func runRuned(ctx context.Context, args []string, stderr io.Writer) int {
	paths, err := bootstrap.Resolve()
	if err != nil {
		fmt.Fprintf(stderr, "rune: cannot resolve home directories: %v\n", err)
		return 1
	}

	// Point runed at the llama-server that `rune install` extracted from the
	// runed full-stack tarball. Without this, runed would re-download
	// llama-server from its own manifest. Set on the process env (rather
	// than via execInstalledBinary's extraEnv) so the supervisor path's
	// re-exec/setsid/fork chain also propagates it to the daemon's child.
	_ = os.Setenv("RUNED_LLAMA_SERVER", paths.LlamaServer)

	detach, forwardedArgs := extractDetachFlag(args)
	if !detach {
		return execInstalledBinary(ctx, paths.RunedBin, "runed", forwardedArgs, nil, stderr)
	}

	if err := paths.EnsureDirs(); err != nil {
		fmt.Fprintf(stderr, "rune: ensure dirs: %v\n", err)
		return 1
	}
	if err := supervisor.EnsureLogDir(paths.DaemonLog); err != nil {
		fmt.Fprintf(stderr, "rune: prepare log dir: %v\n", err)
		return 1
	}

	cfg := supervisor.Config{
		RunedBinary: paths.RunedBinary,
		RunedArgs:   forwardedArgs,
		LogPath:     paths.DaemonLog,
		LockPath:    paths.SupervisorLock,
	}
	if err := supervisor.RunDetached(ctx, cfg); err != nil {
		fmt.Fprintf(stderr, "rune: supervisor: %v\n", err)
		return 1
	}

	return 0
}

func extractDetachFlag(args []string) (detach bool, rest []string) {
	rest = make([]string, 0, len(args))
	for _, a := range args {
		if a == "--detach" || a == "-detach" {
			detach = true
			continue
		}
		rest = append(rest, a)
	}

	return detach, rest
}
