package main

import (
	"fmt"
	"io"
	"os"

	"github.com/CryptoLabInc/rune-cli/internal/bootstrap"
)

func runVersion(w io.Writer) int {
	fmt.Fprintf(w, "rune %s\n", runeVersion)
	manifest := manifestURL
	if manifest == "" {
		manifest = os.Getenv("RUNE_MANIFEST")
	}

	if manifest != "" {
		fmt.Fprintf(w, "manifest: %s\n", manifest)
	} else {
		fmt.Fprintln(w, "manifest missing: supply --manifest-url or RUNE_MANIFEST")
	}

	// Show latest installed rune-mcp/runed (skip if not installed yet)
	if paths, err := bootstrap.Resolve(); err == nil {
		if rec, rerr := bootstrap.ReadInstalledManifest(paths); rerr == nil && rec != nil {
			if rec.RuneMCPVersion != "" {
				fmt.Fprintf(w, "rune-mcp: %s\n", rec.RuneMCPVersion)
			}
			if rec.RunedVersion != "" {
				fmt.Fprintf(w, "runed: %s\n", rec.RunedVersion)
			}
			if rec.InstalledAt != "" {
				fmt.Fprintf(w, "installed: %s\n", rec.InstalledAt)
			}
		}
	}

	return 0
}
