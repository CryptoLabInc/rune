package service

import "time"

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
// Hints (Python exact strings, keep bit-identical for schema):
//   - connection_refused: "Check that envector Cloud is reachable (network / endpoint)."
//   - auth_failure:       "Check envector API key in Vault bundle."
//   - deadline_exceeded:  "envector RPC timed out; server may be overloaded."
//   - timeout:            "Health check timed out after N s."
//   - unknown:            "Unexpected envector error; see logs."
func ClassifyEnvectorError(err error, elapsed time.Duration) (EnvectorErrorType, string) {
	// TODO:
	//  st, ok := status.FromError(err)
	//  if !ok → unknown
	//  switch st.Code() {
	//  case codes.Unavailable:       → connection_refused
	//  case codes.Unauthenticated:   → auth_failure
	//  case codes.DeadlineExceeded:  → deadline_exceeded
	//  default:                      → unknown
	//  }
	_ = err
	_ = elapsed
	return EnvErrUnknown, ""
}
