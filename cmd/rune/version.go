package main

import (
	"fmt"
	"io"
)

func runVersion(w io.Writer) int {
	fmt.Fprintf(w, "rune %s\n", runeVersion)
	if manifestURL != "" {
		fmt.Fprintf(w, "manifest: %s\n", manifestURL)
	} else {
		fmt.Fprintln(w, "manifest missing: supply --manifest-url or RUNE_MANIFEST")
	}

	return 0
}
