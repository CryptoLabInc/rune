package main

import (
	"fmt"
	"io"
	"os"
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

	return 0
}
