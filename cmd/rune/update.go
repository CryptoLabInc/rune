package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/CryptoLabInc/rune-cli/internal/bootstrap"
)

func runUpdate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(stderr)

	check := fs.Bool("check", false, "report available updates without applying")
	jsonOut := fs.Bool("json", false, "emit JSON")
	manifest := fs.String("manifest-url", manifestURL, "override manifest URL")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "rune update: unexpected argument: %v\n", fs.Args())
		return 2
	}

	// Fall-back
	if *manifest == "" {
		if env := os.Getenv("RUNE_MANIFEST"); env != "" {
			*manifest = env
		}
	}
	if *manifest == "" {
		fmt.Fprintln(stderr, "rune update: no manifest URL configured (set --manifest-url or RUNE_MANIFEST)")
		return 2
	}

	plan, err := bootstrap.CheckUpdate(ctx, *manifest, nil)
	if err != nil {
		fmt.Fprintf(stderr, "rune update: %v\n", err)
		return 1
	}

	if *check {
		return reportUpdatePlan(stdout, plan, *jsonOut)
	}

	return applyUpdate(ctx, *manifest, plan, stdout, stderr, *jsonOut)
}

func reportUpdatePlan(w io.Writer, plan *bootstrap.UpdateList, jsonOut bool) int {
	if jsonOut {
		_ = json.NewEncoder(w).Encode(plan)
		return 0
	}

	if !plan.HasUpdates() {
		fmt.Fprintln(w, "rune: all binaries are up to date")
		return 0
	}

	fmt.Fprintln(w, "Available updates:")
	for _, a := range plan.Outdated() {
		fmt.Fprintf(w, "  %s: %s -> %s\n", a.Step, a.Installed, a.Available)
	}

	return 0
}

type updateSummary struct {
	Applied  []appliedUpdate `json:"applied"`
	Deferred []string        `json:"deferred,omitempty"`
	Error    string          `json:"error,omitempty"`
}

type appliedUpdate struct {
	Step string `json:"step"`
	From string `json:"from"`
	To   string `json:"to"`
}

func applyUpdate(ctx context.Context, manifest string, plan *bootstrap.UpdateList, stdout, stderr io.Writer, jsonOut bool) int {
	out := updateSummary{Applied: []appliedUpdate{}}

	logf := func(string, ...any) {}
	if !jsonOut {
		logf = func(format string, a ...any) { fmt.Fprintf(stderr, format+"\n", a...) }
	}

	if !plan.HasUpdates() {
		if jsonOut {
			_ = json.NewEncoder(stdout).Encode(out)
		} else {
			fmt.Fprintln(stdout, "rune: all binaries are up to date")
		}
		return 0
	}

	exit := 0
	for _, a := range plan.Outdated() {
		switch a.Step {
		case bootstrap.StepRuneMCP:
			to, err := bootstrap.UpdateArtifact(ctx, manifest, a.Step, logf)
			if err != nil {
				out.Error = err.Error()
				exit = 1

				if !jsonOut {
					fmt.Fprintf(stderr, "rune update: %s: %v\n", a.Step, err)
				}

				continue
			}

			out.Applied = append(out.Applied, appliedUpdate{Step: a.Step, From: a.Installed, To: to})
			if !jsonOut {
				fmt.Fprintf(stdout, "updated %s: %s -> %s (applies on the next session; run /mcp to reconnect now)\n", a.Step, a.Installed, to)
			}
		case bootstrap.StepRuned:
			// TODO: live daemon update
			out.Deferred = append(out.Deferred, a.Step)
			if !jsonOut {
				fmt.Fprintf(stdout, "%s: %s -> %s available (not applied; live daemon update not yet implemented)\n", a.Step, a.Installed, a.Available)
			}
		}
	}

	if jsonOut {
		_ = json.NewEncoder(stdout).Encode(out)
	}

	return exit
}
