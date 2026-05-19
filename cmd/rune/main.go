// CLI installer and diagnostics tool for Rune
//
// Subcommands:
//	rune install [--force] [--json] [--manifest-url URL]
//	    Download the runed bundle (runed daemon + llama-server) from
//	    GitHub Releases, extract it into ~/.runed/bin/
//      Runed handles its own model on startup
//
//	rune verify [--json]
//	    Read-only health checks
//
//	rune version

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// Configurable at build time via `-ldflags -X main.runeVersion=...`
var runeVersion = "v0.4.0-dev"

// Configurable at build time via `-ldflags -X main.manifestURL=...`
var manifestURL = ""

func main() {
	if len(os.Args) < 2 {
		printHelp(os.Stderr)
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	switch cmd {
	case "install":
		os.Exit(runInstall(ctx, args, os.Stdout, os.Stderr))
	case "verify":
		os.Exit(runVerify(ctx, args, os.Stdout))
	case "version":
		os.Exit(runVersion(os.Stdout))
	case "-h", "--help", "help":
		printHelp(os.Stdout)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "rune: unknown command %q\n\n", cmd)
		printHelp(os.Stderr)
		os.Exit(2)
	}
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `Rune plugin CLI

Usage:
  rune install [--force] [--json] [--manifest-url URL]
        download the runed bundle from GitHub releases into ~/.runed/bin/
        (next: /rune:configure + /rune:activate to finish setup)
  rune verify [--json]
        run health checks
  rune version
        print version and manifest URL

Environment:
  RUNE_HOME       override ~/.rune/  (rune-mcp's realm)
  RUNED_HOME      override ~/.runed/ (runed daemon's realm)
  RUNE_MANIFEST   override the build-baked manifest URL (mostly for tests)
`)
}
