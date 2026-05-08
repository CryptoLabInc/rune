package envector

import (
	"errors"
	"testing"

	envector "github.com/CryptoLabInc/envector-go-sdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// XXX: These tests run without CGO / server
func TestMapSDKError_Nil(t *testing.T) {
	if got := MapSDKError(nil); got != nil {
		t.Fatalf("MapSDKError(nil) = %v, want nil", got)
	}
}

func TestMapSDKError_SDKSentinels(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantCode  string
		wantRetry bool
	}{
		{
			name:      "ErrKeysNotForEncrypt",
			err:       envector.ErrKeysNotForEncrypt,
			wantCode:  "DECRYPTOR_UNAVAILABLE",
			wantRetry: false,
		},
		{
			name:      "ErrKeysNotForDecrypt",
			err:       envector.ErrKeysNotForDecrypt,
			wantCode:  "DECRYPTOR_UNAVAILABLE",
			wantRetry: false,
		},
		{
			name:      "ErrKeysNotForRegister",
			err:       envector.ErrKeysNotForRegister,
			wantCode:  "KEY_NOT_FOR_REGISTER",
			wantRetry: false,
		},
		{
			name:      "ErrClientClosed",
			err:       envector.ErrClientClosed,
			wantCode:  "ENVECTOR_CONNECTION_LOST",
			wantRetry: true,
		},
		{
			name:      "ErrKeysNotFound",
			err:       envector.ErrKeysNotFound,
			wantCode:  "ENVECTOR_KEYS_NOT_FOUND",
			wantRetry: false,
		},
		{
			name:      "ErrKeysRequired",
			err:       envector.ErrKeysRequired,
			wantCode:  "ENVECTOR_KEYS_REQUIRED",
			wantRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapSDKError(tt.err)
			var ae *Error
			if !errors.As(got, &ae) {
				t.Fatalf("MapSDKError(%v) is not *Error: %T", tt.err, got)
			}
			if ae.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", ae.Code, tt.wantCode)
			}
			if ae.Retryable != tt.wantRetry {
				t.Errorf("Retryable = %v, want %v", ae.Retryable, tt.wantRetry)
			}
			if ae.Cause != tt.err {
				t.Errorf("Cause = %v, want %v", ae.Cause, tt.err)
			}
		})
	}
}

func TestMapSDKError_GRPCCodes(t *testing.T) {
	tests := []struct {
		name      string
		code      codes.Code
		wantCode  string
		wantRetry bool
	}{
		{"Unavailable", codes.Unavailable, "ENVECTOR_CONNECTION_LOST", true},
		{"DeadlineExceeded", codes.DeadlineExceeded, "ENVECTOR_CONNECTION_LOST", true},
		{"ResourceExhausted", codes.ResourceExhausted, "ENVECTOR_CONNECTION_LOST", true},
		{"Unauthenticated", codes.Unauthenticated, "ENVECTOR_AUTH_FAILED", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := status.Error(tt.code, "test message")
			got := MapSDKError(err)
			var ae *Error
			if !errors.As(got, &ae) {
				t.Fatalf("MapSDKError(status %s) is not *Error: %T", tt.code, got)
			}
			if ae.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", ae.Code, tt.wantCode)
			}
			if ae.Retryable != tt.wantRetry {
				t.Errorf("Retryable = %v, want %v", ae.Retryable, tt.wantRetry)
			}
		})
	}
}

func TestMapSDKError_UnknownGRPCCode(t *testing.T) {
	err := status.Error(codes.PermissionDenied, "forbidden")
	got := MapSDKError(err)
	var ae *Error
	if !errors.As(got, &ae) {
		t.Fatalf("MapSDKError(PermissionDenied) is not *Error: %T", got)
	}
	if ae.Code != "ENVECTOR_INTERNAL" {
		t.Errorf("Code = %q, want ENVECTOR_INTERNAL", ae.Code)
	}
	if ae.Retryable {
		t.Error("expected Retryable=false for unmapped gRPC code")
	}
}

func TestMapSDKError_GenericError(t *testing.T) {
	err := errors.New("something broke")
	got := MapSDKError(err)
	var ae *Error
	if !errors.As(got, &ae) {
		t.Fatalf("MapSDKError(generic) is not *Error: %T", got)
	}
	if ae.Code != "ENVECTOR_INTERNAL" {
		t.Errorf("Code = %q, want ENVECTOR_INTERNAL", ae.Code)
	}
	if ae.Retryable {
		t.Error("expected Retryable=false for generic error")
	}
}

// --- Error types ---//
func TestError_ErrorString(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{"code only", &Error{Code: "TEST"}, "TEST"},
		{"code + message", &Error{Code: "TEST", Message: "details"}, "TEST: details"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := &Error{Code: "TEST", Cause: cause}
	if !errors.Is(err, cause) {
		t.Error("errors.Is should find the cause through Unwrap")
	}
}

func TestError_UnwrapNil(t *testing.T) {
	err := &Error{Code: "TEST"}
	if err.Unwrap() != nil {
		t.Error("Unwrap should return nil when Cause is nil")
	}
}
