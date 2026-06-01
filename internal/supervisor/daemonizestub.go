//go:build !unix

package supervisor

import (
	"errors"
	"os/exec"
)

// Stubs for non-unix builds
func applyDetachAttrs(*exec.Cmd) {}

func redirectStdio(string) error {
	return errors.New("supervisor: detached mode is not supported on this platform")
}
