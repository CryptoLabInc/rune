package embedder

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// retry executes call() with the D7 backoff schedule [0, 500ms, 2s].
//
// Spec: docs/v04/spec/components/embedder.md §Retry 정책 (D7).
// Total attempts: 3 (one per RetryBackoffs entry).
//
// Retryable gRPC codes:
//
//	Unavailable          — daemon restart / transient network
//	DeadlineExceeded     — per-call timeout hit
//	ResourceExhausted    — daemon overloaded
//
// Non-retryable errors (e.g., InvalidArgument) return immediately.
//
// On any non-nil return, errors are wrapped via MapGRPCError so the service
// layer can detect adapter retryable failures (typed embedder.Error with Retryable=true)
func retry[R any](ctx context.Context, call func(context.Context) (R, error)) (R, error) {
	var zero R
	var lastErr error
	for _, delay := range RetryBackoffs {
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return zero, MapGRPCError(ctx.Err())
			}
		}
		r, err := call(ctx)
		if err == nil {
			return r, nil
		}
		if !retryable(err) {
			return zero, MapGRPCError(err)
		}
		lastErr = err
	}
	return zero, MapGRPCError(fmt.Errorf("embedder: all retries exhausted: %w", lastErr))
}

// retryable returns true for transient gRPC codes.
//
//	Unavailable / DeadlineExceeded / ResourceExhausted → true
//	other (including non-gRPC errors)                   → false
func retryable(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
		return true
	}
	return false
}
