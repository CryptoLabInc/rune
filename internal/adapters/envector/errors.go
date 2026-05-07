package envector

import (
	"errors"

	envector "github.com/CryptoLabInc/envector-go-sdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Error — envector adapter's typed error. Wraps an SDK or gRPC cause.
// Service layer catches these and converts to domain.RuneError for MCP responses.
// Spec: docs/v04/spec/components/envector.md §에러 처리.
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

// Sentinel errors — spec §에러 처리.
var (
	// Connection / transport.
	ErrConnectionLost = &Error{Code: "ENVECTOR_CONNECTION_LOST", Retryable: true}

	// PR-conditional: SDK OpenKeysFromFile without SecKey.json (Q4).
	ErrDecryptorUnavailable = &Error{Code: "DECRYPTOR_UNAVAILABLE", Retryable: false}

	// Atomicity violation detected (len(ItemIDs) != len(Vectors)) — D17 probe trigger.
	ErrInsertInconsistent = &Error{Code: "ENVECTOR_INCONSISTENT", Retryable: false}
)

// MapSDKError converts an envector-go SDK error (or underlying gRPC status)
// into an adapter-level Error. Service layer should subsequently wrap to domain.RuneError.
//
// Priority: SDK typed errors -> gRPC status codes -> generic fallback
func MapSDKError(err error) error {
	if err == nil {
		return nil
	}

	// SDK typed errors
	if errors.Is(err, envector.ErrKeysNotForEncrypt) {
		return &Error{Code: ErrDecryptorUnavailable.Code, Message: err.Error(), Retryable: false, Cause: err}
	}
	if errors.Is(err, envector.ErrKeysNotForDecrypt) {
		return &Error{Code: ErrDecryptorUnavailable.Code, Message: err.Error(), Retryable: false, Cause: err}
	}
	if errors.Is(err, envector.ErrKeysNotForRegister) {
		return &Error{Code: "KEY_NOT_FOR_REGISTER", Message: err.Error(), Retryable: false, Cause: err}
	}
	if errors.Is(err, envector.ErrClientClosed) {
		return &Error{Code: ErrConnectionLost.Code, Message: err.Error(), Retryable: true, Cause: err}
	}
	if errors.Is(err, envector.ErrKeysNotFound) {
		return &Error{Code: "ENVECTOR_KEYS_NOT_FOUND", Message: err.Error(), Retryable: false, Cause: err}
	}
	if errors.Is(err, envector.ErrKeysRequired) {
		return &Error{Code: "ENVECTOR_KEYS_REQUIRED", Message: err.Error(), Retryable: false, Cause: err}
	}

	// gRPC status codes
	st, ok := status.FromError(err)
	if ok {
		switch st.Code() {
		case codes.Unavailable, codes.DeadlineExceeded:
			return &Error{
				Code:      ErrConnectionLost.Code,
				Message:   st.Message(),
				Retryable: true,
				Cause:     err,
			}
		case codes.ResourceExhausted:
			return &Error{
				Code:      ErrConnectionLost.Code,
				Message:   "resource exhausted: " + st.Message(),
				Retryable: true,
				Cause:     err,
			}
		case codes.Unauthenticated:
			return &Error{
				Code:      "ENVECTOR_AUTH_FAILED",
				Message:   st.Message(),
				Retryable: false,
				Cause:     err,
			}
		}
	}

	// Default: non-retryable internal error
	return &Error{
		Code:      "ENVECTOR_INTERNAL",
		Message:   err.Error(),
		Retryable: false,
		Cause:     err,
	}
}
