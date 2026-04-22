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
//
// Some map 1:1 from envector-go SDK typed errors (`ErrKeysAlreadyActivated`,
// `ErrKeysNotFound`, etc.); others are adapter-level composites.
var (
	// Connection / transport.
	ErrConnectionLost = &Error{Code: "ENVECTOR_CONNECTION_LOST", Retryable: true}

	// ActivateKeys race (Q3 — server-side idempotency assumed MVP).
	ErrKeyActivationConflict = &Error{Code: "KEY_ACTIVATION_CONFLICT", Retryable: true}

	// PR-conditional: SDK OpenKeysFromFile without SecKey.json (Q4).
	// Surfaces only after SDK condition relaxation PR is merged.
	ErrDecryptorUnavailable = &Error{Code: "DECRYPTOR_UNAVAILABLE", Retryable: false}

	// Atomicity violation detected (len(ItemIDs) != len(Vectors)) — D17 probe trigger.
	ErrInsertInconsistent = &Error{Code: "ENVECTOR_INCONSISTENT", Retryable: false}
)

// MapSDKError converts an envector-go SDK error (or underlying gRPC status)
// into an adapter-level Error. Service layer should subsequently wrap to domain.RuneError.
//
// Python 11 CONNECTION_ERROR_PATTERNS (envector_sdk.py:L89-101) are NOT ported.
// Go relies on SDK typed errors (`errors.Is`) + gRPC status codes — see
// spec/components/envector.md "Python 대비 (의도적 차이)".
//
// TODO: implement with errors.Is(err, envectorsdk.ErrKeysAlreadyActivated) etc.
// + google.golang.org/grpc/status for code-based dispatch.
func MapSDKError(err error) error {
	if err == nil {
		return nil
	}
	// TODO:
	//   if errors.Is(err, sdk.ErrKeysAlreadyActivated) → ErrKeyActivationConflict
	//   status.Code() switch:
	//     Unavailable / DeadlineExceeded → ErrConnectionLost (retryable)
	//     Unauthenticated → a non-retryable adapter error (auth)
	//     ResourceExhausted → retryable
	return err
}
