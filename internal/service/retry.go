package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/envector/rune-go/internal/adapters/envector"
	"github.com/envector/rune-go/internal/lifecycle"
)

const retriggerSettleTimeout = 30 * time.Second

// Re-trigger boot loop if enVector connection failure occurred since it might be
// caused by outdated configurations
func withEnvectorRetry[T any](
	ctx context.Context,
	mgr *lifecycle.Manager,
	op string,
	fn func() (T, error),
) (T, error) {
	out, err := fn()
	if err == nil {
		return out, nil
	}
	if mgr == nil || !envector.IsRetryable(err) {
		return out, err
	}

	slog.Warn("envector call failed with retryable error — re-bootstrapping",
		"op", op,
		"err", err)

	mgr.Retrigger()
	if !waitForActiveAfterRetrigger(ctx, mgr, retriggerSettleTimeout) {
		var zero T
		return zero, fmt.Errorf("envector %s: re-boot did not settle to active: %w", op, err)
	}

	return fn()
}

func waitForActiveAfterRetrigger(ctx context.Context, mgr *lifecycle.Manager, timeout time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(500 * time.Millisecond):
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		switch mgr.Current() {
		case lifecycle.StateActive:
			return true
		case lifecycle.StateDormant:
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
	return false
}
