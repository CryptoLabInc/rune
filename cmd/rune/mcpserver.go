package main

import (
	"context"
	"fmt"
	"io"

	"github.com/CryptoLabInc/rune-cli/internal/bootstrap"
)

func runMCPServer(ctx context.Context, args []string, stderr io.Writer) int {
	paths, err := bootstrap.Resolve()
	if err != nil {
		fmt.Fprintf(stderr, "rune: cannot resolve home directories: %v\n", err)
		return 1
	}

	return execInstalledBinary(ctx, paths.RuneBin, "rune-mcp", args, stderr)
}
