package envector

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

	// ActivateKeys race (Q3 — server-side idempotency assumed MVP).
	ErrKeyActivationConflict = &Error{Code: "KEY_ACTIVATION_CONFLICT", Retryable: true}

	// PR-conditional: SDK OpenKeysFromFile without SecKey.json (Q4).
	ErrDecryptorUnavailable = &Error{Code: "DECRYPTOR_UNAVAILABLE", Retryable: false}

	// Atomicity violation detected (len(ItemIDs) != len(Vectors)) — D17 probe trigger.
	ErrInsertInconsistent = &Error{Code: "ENVECTOR_INCONSISTENT", Retryable: false}
)

// MapSDKError converts an envector-go SDK error (or underlying gRPC status)
// into an adapter-level Error. Service layer should subsequently wrap to domain.RuneError.
//
// Go relies on SDK typed errors (`errors.Is`) + gRPC status codes — see
// spec/components/envector.md "Python 대비 (의도적 차이)".
func MapSDKError(err error) error {
	if err == nil {
		return nil
	}

	// Try gRPC status extraction
	st, ok := statusFromError(err)
	if ok {
		switch st.code {
		case codeUnavailable, codeDeadlineExceeded:
			return &Error{
				Code:      ErrConnectionLost.Code,
				Message:   st.message,
				Retryable: true,
				Cause:     err,
			}
		case codeResourceExhausted:
			return &Error{
				Code:      ErrConnectionLost.Code,
				Message:   "resource exhausted: " + st.message,
				Retryable: true,
				Cause:     err,
			}
		case codeUnauthenticated:
			return &Error{
				Code:      "ENVECTOR_AUTH_FAILED",
				Message:   st.message,
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

type grpcStatus struct {
	code    int
	message string
}

// gRPC constants
const (
	codeUnauthenticated   = 16
	codeUnavailable       = 14
	codeDeadlineExceeded  = 4
	codeResourceExhausted = 8
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
