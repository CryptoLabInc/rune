package service

import (
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EnvectorErrorType — envector probe error classification.
// Python: server.py:L655-672 (string pattern matching — Python).
// Go: gRPC status.Code() enum based (spec/components/envector.md "의도적 차이").
type EnvectorErrorType string

const (
	EnvErrConnectionRefused EnvectorErrorType = "connection_refused"
	EnvErrAuthFailure       EnvectorErrorType = "auth_failure"
	EnvErrDeadlineExceeded  EnvectorErrorType = "deadline_exceeded"
	EnvErrTimeout           EnvectorErrorType = "timeout" // context.WithTimeout deadline
	EnvErrUnknown           EnvectorErrorType = "unknown"
)

// ClassifyEnvectorError maps an error (with its elapsed latency) to a typed
// classification + user-facing hint. Used by LifecycleService.Diagnostics.
//
// Python match (server.py:L655-672):
//
//	UNAVAILABLE | Connection refused → connection_refused
//	UNAUTHENTICATED | 401             → auth_failure
//	DEADLINE_EXCEEDED                  → deadline_exceeded
//	other                              → unknown
//
// Hints (Python exact strings, keep bit-identical for schema)
// XXX: it seems that ErrDeadlineExcceded can cover ErrTimeout
func ClassifyEnvectorError(err error, elapsed time.Duration) (EnvectorErrorType, string) {
	st, ok := status.FromError(err)
	if !ok {
		return EnvErrUnknown, fmt.Sprintf("Unexpected envector error (%.1fms): %v", float64(elapsed.Milliseconds()), err)
	}

	switch st.Code() {
	case codes.Unavailable:
		return EnvErrConnectionRefused, "Check that the enVector endpoint is correct and reachable from this machine"
	case codes.Unauthenticated:
		return EnvErrAuthFailure, "enVector API key may be invalid or expired"
	case codes.DeadlineExceeded:
		return EnvErrDeadlineExceeded, "The enVector gRPC deadline was exceeded. Run /rune:activate to pre-warm, then retry /rune:status"
	default:
		return EnvErrUnknown, "Run /rune:activate to reinitialize the connection, or check network connectivity"
	}
}
