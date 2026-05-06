package vault

import "errors"

// Error — vault adapter's typed error. Wraps a cause (gRPC error or IO error).
// Service layer catches these and converts to domain.RuneError for MCP responses.
// Spec: docs/v04/spec/components/vault.md §에러 분류.
type Error struct {
	Code      string
	Message   string
	Retryable bool
	Cause     error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

// Unwrap allows errors.Is / errors.As to inspect the cause.
func (e *Error) Unwrap() error { return e.Cause }

// Sentinel errors — vault.md §에러 분류 L275-283.
var (
	ErrVaultUnavailable = &Error{Code: "VAULT_UNAVAILABLE", Retryable: true}
	ErrVaultAuthFailed  = &Error{Code: "VAULT_AUTH_FAILED", Retryable: false}
	ErrVaultKeyNotFound = &Error{Code: "VAULT_KEY_NOT_FOUND", Retryable: false}
	ErrVaultInternal    = &Error{Code: "VAULT_INTERNAL", Retryable: true}
	ErrVaultTimeout     = &Error{Code: "VAULT_TIMEOUT", Retryable: true}

	// ErrNotHTTPScheme — returned by HealthFallback when endpoint is not http(s).
	ErrNotHTTPScheme = errors.New("vault: endpoint not http(s) scheme")
)

// MapGRPCError maps a gRPC status error to the appropriate vault sentinel + cause.
//
// gRPC → sentinel (spec §에러 분류 L286-290):
//
//	Unauthenticated     → ErrVaultAuthFailed
//	NotFound            → ErrVaultKeyNotFound
//	Unavailable         → ErrVaultUnavailable
//	DeadlineExceeded    → ErrVaultTimeout
//	<other / non-gRPC>  → ErrVaultInternal
//
// Returns nil for nil input.
func MapGRPCError(err error) error {
	if err == nil {
		return nil
	}

	st, ok := statusFromError(err)
	if !ok {
		return &Error{
			Code:      ErrVaultInternal.Code,
			Message:   err.Error(),
			Retryable: true,
			Cause:     err,
		}
	}

	switch st.code {
	case codeUnauthenticated:
		return &Error{
			Code:      ErrVaultAuthFailed.Code,
			Message:   st.message,
			Retryable: false,
			Cause:     err,
		}
	case codeNotFound:
		return &Error{
			Code:      ErrVaultKeyNotFound.Code,
			Message:   st.message,
			Retryable: false,
			Cause:     err,
		}
	case codeUnavailable:
		return &Error{
			Code:      ErrVaultUnavailable.Code,
			Message:   st.message,
			Retryable: true,
			Cause:     err,
		}
	case codeDeadlineExceeded:
		return &Error{
			Code:      ErrVaultTimeout.Code,
			Message:   st.message,
			Retryable: true,
			Cause:     err,
		}
	default:
		return &Error{
			Code:      ErrVaultInternal.Code,
			Message:   st.message,
			Retryable: true,
			Cause:     err,
		}
	}
}

type grpcStatus struct {
	code    int
	message string
}

// ref: google.golang.org/grpc/codes
const (
	codeUnauthenticated  = 16
	codeNotFound         = 5
	codeUnavailable      = 14
	codeDeadlineExceeded = 4
)

func statusFromError(err error) (grpcStatus, bool) {
	type grpcStatuser interface {
		GRPCStatus() interface {
			Code() int
			Message() string
		}
	}

	if gs, ok := err.(grpcStatuser); ok {
		st := gs.GRPCStatus()
		return grpcStatus{code: st.Code(), message: st.Message()}, true
	}

	return grpcStatus{}, false
}
