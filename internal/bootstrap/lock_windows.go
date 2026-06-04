//go:build windows

package bootstrap

import (
	"context"
	"errors"
	"time"
)

const InstallLockTimeout = 30 * time.Second

func acquireInstallLock(ctx context.Context, lockPath string, timeout time.Duration) (func(), error) {
	return nil, errors.New("install: Windows support not in v0.4.0")
}
