//go:build unix

package supervisor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Open lockPath and take exclusive flock(2), returns:
//   - (file, true, nil): lock acquired, caller must Close() to release
//   - (nil, false, nil): another process already hold lock
//   - (nil, false, err): unexpected error
func acquireSupervisorLock(lockPath string) (*os.File, bool, error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, false, fmt.Errorf("mkdir parent: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open %s: %w", lockPath, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("flock %s: %w", lockPath, err)
	}

	return f, true, nil
}
