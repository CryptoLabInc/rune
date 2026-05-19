package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/envector/rune-go/internal/bootstrap"
)

func runVerify(ctx context.Context, args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stdout)
	jsonOut := fs.Bool("json", false, "emit JSON instead of human-readable text")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	result := bootstrap.RunInstallChecks(ctx)

	if *jsonOut {
		_ = json.NewEncoder(stdout).Encode(result)
	} else {
		renderVerifyText(stdout, result)
	}

	if !result.OK {
		return 1
	}
	return 0
}

func renderVerifyText(w io.Writer, r *bootstrap.InstallChecks) {
	for _, c := range r.Checks {
		fmt.Fprintf(w, "%s %s\n", statusSymbol(c.Status), c.Name)
		if c.Detail != "" {
			fmt.Fprintf(w, "    %s\n", c.Detail)
		}
		if c.FixHint != "" {
			fmt.Fprintf(w, "    -> %s\n", c.FixHint)
		}
	}

	fmt.Fprintln(w)
	if r.OK {
		fmt.Fprintln(w, "status: OK")
	} else {
		fmt.Fprintln(w, "status: FAIL")
	}
}

func statusSymbol(s string) string {
	switch s {
	case bootstrap.StatusOK:
		return "[ok]  "
	case bootstrap.StatusWarn:
		return "[warn]"
	case bootstrap.StatusFail:
		return "[fail]"
	default:
		return "[?]   "
	}
}
