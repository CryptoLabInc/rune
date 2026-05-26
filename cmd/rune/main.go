// CLI installer and diagnostics tool for Rune
//
// `rune install` downloads two binaries from GitHub Releases and places
// them under the home realms they belong to:
//
//	~/.rune/bin/rune-mcp (rune plugin realm)
//	~/.runed/bin/runed   (runed daemon realm)
//
// Subcommands:
//
//	rune install [--force] [--json] [--manifest-url URL]
//	    Fetch the manifest, and place the rune-mcp and runed binaries
//
//	rune verify [--json]
//	    Read-only health checks
//
//	rune version
//	    Print the rune CLI version and the manifest URL
//
//	rune mcp-server [args...]
//      The plugin manifest's mcpServers entry uses this to spawn mcp server without absolute path in plugin.json

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
	case "mcp-server":
		os.Exit(runMCPServer(ctx, args, os.Stderr))
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
        download rune-mcp into ~/.rune/bin/ and runed into ~/.runed/bin/
        (next: /rune:configure + /rune:activate to finish setup)
  rune verify [--json]
        run read-only health checks
  rune version
        print version and manifest URL
  rune mcp-server [args...]
        forward stdio to ~/.rune/bin/rune-mcp (plugin-manifest entry point)

Environment:
  RUNE_HOME       override ~/.rune/  (rune plugin realm: config + rune-mcp)
  RUNED_HOME      override ~/.runed/ (runed daemon realm)
  RUNE_MANIFEST   override the build-baked manifest URL (mostly for tests)
`)
}
