package embedder

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Mirros vault.Error and envectorError
type Error struct {
	Code      string
	Message   string
	Retryable bool
	Cause     error
}

func (e *Error) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

var (
	// Retryable
	ErrEmbedderUnavailable = &Error{Code: "EMBEDDER_UNAVAILABLE", Retryable: true} // daemon down or connection fail
	ErrEmbedderTimeout     = &Error{Code: "EMBEDDER_TIMEOUT", Retryable: true}

	// Non-retryable
	ErrEmbedderInternal = &Error{Code: "EMBEDDER_INTERNAL", Retryable: false}
)

// Converts gRPC status error into typed embedder.Error
func MapGRPCError(err error) error {
	if err == nil {
		return nil // nil-safe
	}

	st, ok := status.FromError(err)
	if !ok {
		return &Error{
			Code:      ErrEmbedderInternal.Code,
			Message:   err.Error(),
			Retryable: false,
			Cause:     err,
		}
	}

	switch st.Code() {
	case codes.Unavailable, codes.ResourceExhausted:
		return &Error{
			Code:      ErrEmbedderUnavailable.Code,
			Message:   st.Message(),
			Retryable: true,
			Cause:     err,
		}
	case codes.DeadlineExceeded:
		return &Error{
			Code:      ErrEmbedderTimeout.Code,
			Message:   st.Message(),
			Retryable: true,
			Cause:     err,
		}
	default:
		return &Error{
			Code:      ErrEmbedderInternal.Code,
			Message:   st.Message(),
			Retryable: false,
			Cause:     err,
		}
	}
}

// Expose Error to errors.As via Unwrap
var _ interface{ Unwrap() error } = (*Error)(nil)
