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
// TODO: implement with google.golang.org/grpc/{status,codes}.
//
//	st, ok := status.FromError(err)
//	if !ok { return &Error{Code: ErrVaultInternal.Code, Retryable: true, Cause: err} }
//	switch st.Code() { ... }
func MapGRPCError(err error) error {
	if err == nil {
		return nil
	}
	// TODO: per spec/components/vault.md §에러 분류
	return err
}
