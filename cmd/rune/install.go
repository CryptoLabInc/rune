package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/CryptoLabInc/rune/internal/bootstrap"
)

func runInstall(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "re-download even if binaries are already present")
	jsonOut := fs.Bool("json", false, "emit JSON event per line to stdout instead of text progress to stderr")
	manifest := fs.String("manifest-url", manifestURL, "override the build-baked manifest URL")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Check RUNE_MANIFEST before fail
	if *manifest == "" {
		if env := os.Getenv("RUNE_MANIFEST"); env != "" {
			*manifest = env
		}
	}

	if *manifest == "" {
		const msg = "no manifest URL configured (set --manifest-url or RUNE_MANIFEST)"
		if *jsonOut {
			_ = json.NewEncoder(stdout).Encode(jsonEvent{Event: "summary", Error: msg})
		} else {
			fmt.Fprintln(stderr, "rune install: "+msg)
		}

		return 2
	}

	opts := bootstrap.InstallOptions{
		ManifestURL: *manifest,
		Force:       *force,
	}

	enc := json.NewEncoder(stdout)
	if *jsonOut {
		opts.Log = func(format string, a ...any) {
			_ = enc.Encode(jsonEvent{Event: "log", Message: fmt.Sprintf(format, a...)})
		}
		opts.Progress = func(downloaded, total int64) {
			_ = enc.Encode(jsonEvent{Event: "progress", Downloaded: downloaded, Total: total})
		}
	} else {
		opts.Log = func(format string, a ...any) {
			fmt.Fprintf(stderr, format+"\n", a...)
		}
	}

	result, err := bootstrap.Install(ctx, opts)
	if *jsonOut {
		ev := jsonEvent{Event: "summary"}
		ev.Result = result
		if err != nil {
			ev.Error = err.Error()
		}

		_ = enc.Encode(ev)
	} else {
		if err != nil {
			fmt.Fprintf(stderr, "install failed: %v\n", err)
		} else {
			fmt.Fprintln(stderr, "ready.")
			printNextSteps(stderr)
		}
	}

	if err != nil {
		return 1
	}
	return 0
}

func printNextSteps(stderr io.Writer) {
	if configuredEndpoint() != "" {
		fmt.Fprintln(stderr, "next:")
		fmt.Fprintln(stderr, "  Console credentials are configured - run /rune:activate to make Rune available")
		fmt.Fprintln(stderr, "  (or /rune:status to check health)")
		return
	}

	fmt.Fprintln(stderr, "next:")
	fmt.Fprintln(stderr, "  1. in Claude, run /rune:configure to set up Console credentials")
	fmt.Fprintln(stderr, "  2. then /rune:activate")
}

func configuredEndpoint() string {
	paths, err := bootstrap.Resolve()
	if err != nil {
		return ""
	}

	data, err := os.ReadFile(paths.RuneConfig)
	if err != nil {
		return ""
	}

	var cfg struct {
		Console struct {
			Endpoint string `json:"endpoint"`
		} `json:"console"`
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}

	return cfg.Console.Endpoint
}

type jsonEvent struct {
	Event      string            `json:"event"`
	Message    string            `json:"message,omitempty"`
	Downloaded int64             `json:"downloaded,omitempty"`
	Total      int64             `json:"total,omitempty"`
	Result     *bootstrap.Result `json:"result,omitempty"`
	Error      string            `json:"error,omitempty"`
}
