//go:build !windows

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

const InstallLockTimeout = 30 * time.Second
const installLockPollInterval = 200 * time.Millisecond

func acquireInstallLock(ctx context.Context, lockPath string, timeout time.Duration) (func(), error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", lockPath, err)
	}

	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				_ = f.Close()
			}, nil
		}

		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return nil, fmt.Errorf("flock %s: %w", lockPath, err)
		}
		if !time.Now().Before(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("install lock %s: another install in progress (waited %s)", lockPath, timeout)
		}

		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(installLockPollInterval):
		}
	}
}
