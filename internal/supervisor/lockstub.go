//go:build !unix

package supervisor

import (
	"errors"
	"os"
)

// Stub for non-unix builds
func acquireSupervisorLock(string) (*os.File, bool, error) {
	return nil, false, errors.New("supervisor: file-lock based supervisor is not supported on this platform")
}
