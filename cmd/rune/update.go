package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/CryptoLabInc/rune-cli/internal/bootstrap"
	"github.com/CryptoLabInc/rune-cli/internal/supervisor"
)

func runUpdate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(stderr)

	check := fs.Bool("check", false, "report available updates without applying")
	jsonOut := fs.Bool("json", false, "emit JSON")
	manifest := fs.String("manifest-url", manifestURL, "override manifest URL")
	pluginRoot := fs.String("plugin-root", "", "plugin root for the plugin version check (defaults to $CLAUDE_PLUGIN_ROOT)")
	allowOutdated := fs.Bool("allow-plugin-outdated", false, "apply even if the plugin package is older than the binaries require")
	only := fs.String("only", "", "restrict to a comma-separated set of artifacts: rune_mcp, runed, rune_cli")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "rune update: unexpected argument: %v\n", fs.Args())
		return 2
	}

	onlySteps, err := parseOnly(*only)
	if err != nil {
		fmt.Fprintf(stderr, "rune update: %v\n", err)
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

	mf, err := bootstrap.FetchManifest(ctx, *manifest, nil)
	if err != nil {
		fmt.Fprintf(stderr, "rune update: %v\n", err)
		return 1
	}

	plan, err := bootstrap.PlanFromManifest(mf, runeVersion)
	if err != nil {
		fmt.Fprintf(stderr, "rune update: %v\n", err)
		return 1
	}
	if len(onlySteps) > 0 {
		plan = filterPlan(plan, onlySteps)
	}

	pv, verr := bootstrap.InstalledPluginVersion(*pluginRoot)
	compat := bootstrap.EvaluatePluginCompatibility(pv, mf)
	if verr != nil && mf.MinPluginVersion != "" && !*jsonOut {
		fmt.Fprintf(stderr, "rune update: note: could not read the installed plugin version (%v); skipping the plugin-compatibility check; pass --plugin-root\n", verr)
	}

	if *check {
		return reportUpdatePlan(stdout, plan, compat, *jsonOut)
	}

	return applyUpdate(ctx, *manifest, plan, compat, *allowOutdated, stdout, stderr, *jsonOut)
}

func reportUpdatePlan(w io.Writer, plan *bootstrap.UpdateList, compat bootstrap.PluginCompatibility, jsonOut bool) int {
	if jsonOut {
		_ = json.NewEncoder(w).Encode(plan)
		return 0
	}

	if !plan.HasUpdates() {
		fmt.Fprintln(w, "rune: all binaries are up to date")
	} else {
		fmt.Fprintln(w, "Available updates:")
		for _, a := range plan.Outdated() {
			fmt.Fprintf(w, "  %s: %s -> %s\n", a.Step, a.Installed, a.Available)
		}
	}

	reportPluginOutdated(w, compat)
	return 0
}

func reportPluginOutdated(w io.Writer, compat bootstrap.PluginCompatibility) {
	switch {
	case compat.BelowMinimum:
		fmt.Fprintf(w, "plugin outdated: installed %s is BELOW the minimum %s required by these binaries - update the plugin before applying\n", compat.Installed, compat.Minimum)
	case compat.Behind:
		fmt.Fprintf(w, "plugin update available: installed %s, binaries expect %s\n", compat.Installed, compat.Expected)
	}
}

type updateSummary struct {
	Applied  []appliedUpdate                `json:"applied"`
	Deferred []string                       `json:"deferred,omitempty"`
	Error    string                         `json:"error,omitempty"`
	Plugin   *bootstrap.PluginCompatibility `json:"plugin,omitempty"`
}

type appliedUpdate struct {
	Step string `json:"step"`
	From string `json:"from"`
	To   string `json:"to"`
}

func applyUpdate(ctx context.Context, manifest string, plan *bootstrap.UpdateList, compat bootstrap.PluginCompatibility, allowOutdated bool, stdout, stderr io.Writer, jsonOut bool) int {
	out := updateSummary{Applied: []appliedUpdate{}}
	if compat.Known {
		out.Plugin = &compat
	}

	logf := func(string, ...any) {}
	if !jsonOut {
		logf = func(format string, a ...any) { fmt.Fprintf(stderr, format+"\n", a...) }
	}

	if !plan.HasUpdates() {
		if jsonOut {
			_ = json.NewEncoder(stdout).Encode(out)
		} else {
			fmt.Fprintln(stdout, "rune: all binaries are up to date")
			reportPluginOutdated(stdout, compat)
		}
		return 0
	}

	if compat.BelowMinimum && !allowOutdated {
		msg := fmt.Sprintf("plugin package %s is below the minimum %s required by the new binaries", compat.Installed, compat.Minimum)
		out.Error = msg
		if jsonOut {
			_ = json.NewEncoder(stdout).Encode(out)
		} else {
			fmt.Fprintf(stderr, "rune update: refusing to apply: %s.\n", msg)
			fmt.Fprintf(stderr, "  Update the plugin first then re-run.\n")
			fmt.Fprintf(stderr, "  Ignore with --allow-plugin-outdated\n")
		}
		return 1
	}

	paths, err := bootstrap.Resolve()
	if err != nil {
		fmt.Fprintf(stderr, "rune update: %v\n", err)
		return 1
	}

	exit := 0
	for _, a := range plan.Outdated() {
		switch a.Step {
		case bootstrap.StepRuneMCP:
			to, err := bootstrap.UpdateArtifact(ctx, manifest, a.Step, nil, logf)
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
			// Request supervisor to restart with new binary
			reloaded := false
			reload := func() error {
				resp, rerr := supervisor.SupervisorRequest(paths.SupervisorSock, supervisor.Request{Cmd: "reload"})
				switch {
				case errors.Is(rerr, supervisor.ErrNoSupervisor):
					logf("runed staged; no supervisor running - applies on next daemon start")
					return nil
				case rerr != nil:
					return fmt.Errorf("supervisor reload: %v; %s", rerr, runedRecoveryHint(paths))
				case !resp.OK:
					return fmt.Errorf("supervisor reload failed: %s; %s", resp.Error, runedRecoveryHint(paths))
				}
				reloaded = true
				return nil
			}

			to, err := bootstrap.UpdateArtifact(ctx, manifest, a.Step, reload, logf)
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
				if reloaded {
					fmt.Fprintf(stdout, "updated %s: %s -> %s (daemon reloaded)\n", a.Step, a.Installed, to)
				} else {
					fmt.Fprintf(stdout, "updated %s: %s -> %s (staged; applies on next daemon start)\n", a.Step, a.Installed, to)
				}
			}
		case bootstrap.StepRuneCLI:
			to, err := bootstrap.UpdateCLI(ctx, manifest, runeVersion, logf)
			if errors.Is(err, bootstrap.ErrCLIOutdated) {
				out.Deferred = append(out.Deferred, a.Step)
				if !jsonOut {
					fmt.Fprintf(stderr, "rune update: %s: skipped (%v)\n", a.Step, err)
				}

				continue
			}
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
				fmt.Fprintf(stdout, "updated %s: %s -> %s (apply on the next rune execution)\n", a.Step, a.Installed, to)
			}
		}
	}

	if !jsonOut {
		switch {
		case compat.BelowMinimum && allowOutdated:
			fmt.Fprintf(stderr, "warning: applied with --allow-plugin-outdated; plugin %s is below the required %s - update the plugin.\n", compat.Installed, compat.Minimum)
		case compat.Behind:
			fmt.Fprintf(stdout, "note: plugin package is %s; these binaries expect %s. Update plugin to keep commands/agents in sync best.\n", compat.Installed, compat.Expected)
		}
	}

	if jsonOut {
		_ = json.NewEncoder(stdout).Encode(out)
	}

	return exit
}

func runedRecoveryHint(paths *bootstrap.Paths) string {
	return fmt.Sprintf("the daemon may be down - run `%s runed --detach` (or /rune:activate) to restart it", paths.RuneCLIBinary)
}

func parseOnly(only string) (map[string]bool, error) {
	set := map[string]bool{}
	for _, tok := range strings.Split(only, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if tok != bootstrap.StepRuneMCP && tok != bootstrap.StepRuned && tok != bootstrap.StepRuneCLI {
			return nil, fmt.Errorf("--only: unknown artifact %q (want %s, %s or %s)", tok, bootstrap.StepRuneMCP, bootstrap.StepRuned, bootstrap.StepRuneCLI)
		}

		set[tok] = true
	}

	if strings.TrimSpace(only) != "" && len(set) == 0 {
		return nil, fmt.Errorf("--only: no valid artifacts in %q", only)
	}

	return set, nil
}

func filterPlan(plan *bootstrap.UpdateList, want map[string]bool) *bootstrap.UpdateList {
	out := &bootstrap.UpdateList{}
	for _, a := range plan.Artifacts {
		if want[a.Step] {
			out.Artifacts = append(out.Artifacts, a)
		}
	}

	return out
}
