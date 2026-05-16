package lifecycle

import (
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/envector/rune-go/internal/adapters/vault"
	"github.com/envector/rune-go/internal/domain"
)

func TestClassifyBootError_NilReturnsNil(t *testing.T) {
	if got := ClassifyBootError(nil, BootErrCtx{}); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestClassifyBootError_ConfigMissing(t *testing.T) {
	be := ClassifyBootError(fs.ErrNotExist, BootErrCtx{Phase: domain.BootPhaseConfigLoad})
	if be.Kind != domain.BootErrConfigMissing {
		t.Fatalf("kind: got %q want %q", be.Kind, domain.BootErrConfigMissing)
	}
	if !strings.Contains(be.Hint, "/rune:configure") {
		t.Errorf("hint should mention /rune:configure: %q", be.Hint)
	}
	if be.Retryable() {
		t.Error("config missing should not be retryable")
	}
}

func TestClassifyBootError_TypedX509UnknownAuthority(t *testing.T) {
	// x509.UnknownAuthorityError zero value is fine — we only check the type.
	err := x509.UnknownAuthorityError{}
	be := ClassifyBootError(err, BootErrCtx{
		Phase:         domain.BootPhaseVaultManifest,
		VaultEndpoint: "tcp://vault.example:50051",
		VaultCAPath:   "/home/user/.rune/certs/ca.pem",
		Attempts:      3,
	})
	if be.Kind != domain.BootErrVaultTLSHandshake {
		t.Fatalf("kind: got %q want %q", be.Kind, domain.BootErrVaultTLSHandshake)
	}
	if !strings.Contains(be.Hint, "/home/user/.rune/certs/ca.pem") {
		t.Errorf("hint should mention CA path: %q", be.Hint)
	}
	if be.Retryable() {
		t.Error("TLS handshake should not be retryable (won't fix on retry)")
	}
}

func TestClassifyBootError_TLSStringFallback(t *testing.T) {
	// gRPC commonly returns errors like this where the typed x509 error is
	// lost in string-wrapping. The classifier needs to recover via substring.
	err := errors.New(
		"rpc error: code = Unavailable desc = connection error: " +
			"desc = \"transport: authentication handshake failed: " +
			"tls: failed to verify certificate: x509: certificate signed by unknown authority\"",
	)
	be := ClassifyBootError(err, BootErrCtx{
		Phase:         domain.BootPhaseVaultManifest,
		VaultEndpoint: "tcp://158.180.87.178:50051",
		VaultCAPath:   "/u/.rune/certs/ca.pem",
	})
	if be.Kind != domain.BootErrVaultTLSHandshake {
		t.Fatalf("kind: got %q want %q\ndetail: %s", be.Kind, domain.BootErrVaultTLSHandshake, be.Detail)
	}
	if !strings.Contains(be.Hint, "stale") && !strings.Contains(be.Hint, "regenerated") {
		t.Errorf("hint should mention CA regeneration: %q", be.Hint)
	}
}

func TestClassifyBootError_VaultAuth(t *testing.T) {
	// MapGRPCError converts gRPC Unauthenticated → vault.ErrVaultAuthFailed.
	authErr := status.Error(codes.Unauthenticated, "token validation failed")
	mapped := vault.MapGRPCError(authErr)
	be := ClassifyBootError(mapped, BootErrCtx{
		Phase:         domain.BootPhaseVaultManifest,
		VaultEndpoint: "tcp://x:50051",
	})
	if be.Kind != domain.BootErrVaultAuth {
		t.Fatalf("kind: got %q want %q", be.Kind, domain.BootErrVaultAuth)
	}
	if be.Retryable() {
		t.Error("auth failure should not be retryable (won't fix on retry)")
	}
	if !strings.Contains(strings.ToLower(be.Hint), "token") {
		t.Errorf("hint should mention token: %q", be.Hint)
	}
}

func TestClassifyBootError_VaultNetworkViaUnavailable(t *testing.T) {
	netErr := status.Error(codes.Unavailable, "name resolver error: produced zero addresses")
	mapped := vault.MapGRPCError(netErr)
	be := ClassifyBootError(mapped, BootErrCtx{
		Phase:         domain.BootPhaseVaultManifest,
		VaultEndpoint: "tcp://does-not-resolve:50051",
	})
	if be.Kind != domain.BootErrVaultNetwork {
		t.Fatalf("kind: got %q want %q", be.Kind, domain.BootErrVaultNetwork)
	}
	if !be.Retryable() {
		t.Error("network failure should be retryable")
	}
}

func TestClassifyBootError_DNSError(t *testing.T) {
	dnsErr := &net.DNSError{Name: "vault.invalid", Err: "no such host"}
	be := ClassifyBootError(dnsErr, BootErrCtx{
		Phase:         domain.BootPhaseVaultManifest,
		VaultEndpoint: "tcp://vault.invalid:50051",
	})
	if be.Kind != domain.BootErrVaultDNS {
		t.Fatalf("kind: got %q want %q", be.Kind, domain.BootErrVaultDNS)
	}
}

func TestClassifyBootError_GRPCDeadlineExceeded(t *testing.T) {
	be := ClassifyBootError(
		status.Error(codes.DeadlineExceeded, "deadline exceeded"),
		BootErrCtx{Phase: domain.BootPhaseVaultManifest, VaultEndpoint: "x"},
	)
	if be.Kind != domain.BootErrVaultTimeout {
		t.Fatalf("kind: got %q want %q", be.Kind, domain.BootErrVaultTimeout)
	}
}

func TestClassifyBootError_PhaseFallback_EmbedderDial(t *testing.T) {
	// Unrecognized error in embedder_dial phase falls back to embedder_unreachable.
	be := ClassifyBootError(
		fmt.Errorf("some-totally-unknown-embedder-error"),
		BootErrCtx{Phase: domain.BootPhaseEmbedderDial},
	)
	if be.Kind != domain.BootErrEmbedderUnreachable {
		t.Fatalf("kind: got %q want %q", be.Kind, domain.BootErrEmbedderUnreachable)
	}
	if !strings.Contains(be.Hint, "runed") {
		t.Errorf("hint should mention runed daemon: %q", be.Hint)
	}
}

func TestClassifyBootError_PhaseFallback_EnvectorIndex(t *testing.T) {
	be := ClassifyBootError(
		fmt.Errorf("unknown index error"),
		BootErrCtx{Phase: domain.BootPhaseEnvectorIndex},
	)
	if be.Kind != domain.BootErrEnvectorIndex {
		t.Fatalf("kind: got %q want %q", be.Kind, domain.BootErrEnvectorIndex)
	}
}

func TestClassifyBootError_FullyUnknown(t *testing.T) {
	be := ClassifyBootError(
		fmt.Errorf("totally novel error"),
		BootErrCtx{}, // no phase, no context
	)
	if be.Kind != domain.BootErrUnknown {
		t.Fatalf("kind: got %q want %q", be.Kind, domain.BootErrUnknown)
	}
	if be.Detail == "" {
		t.Error("detail should always be populated")
	}
}

func TestClassifyDormantReason(t *testing.T) {
	cases := []struct {
		reason   string
		wantKind domain.BootErrorKind
	}{
		{"not_configured", domain.BootErrConfigMissing},
		{"vault_unconfigured", domain.BootErrVaultNotConfigured},
		{"user_deactivated", domain.BootErrUserDeactivated},
		{"invalid_state", domain.BootErrConfigInvalid},
		{"", domain.BootErrConfigInvalid},
		{"some_unknown_reason", domain.BootErrConfigInvalid},
	}
	for _, c := range cases {
		t.Run(c.reason, func(t *testing.T) {
			be := ClassifyDormantReason(c.reason)
			if be.Kind != c.wantKind {
				t.Errorf("reason %q: kind=%q want=%q", c.reason, be.Kind, c.wantKind)
			}
			if be.Hint == "" {
				t.Errorf("reason %q: hint is empty", c.reason)
			}
		})
	}
}

func TestBootError_RetryableTLS(t *testing.T) {
	// TLS handshake: NOT retryable (won't fix on retry without user action).
	be := &domain.BootError{Kind: domain.BootErrVaultTLSHandshake}
	if be.Retryable() {
		t.Error("TLS handshake should be non-retryable")
	}

	// Network: IS retryable.
	be = &domain.BootError{Kind: domain.BootErrVaultNetwork}
	if !be.Retryable() {
		t.Error("network should be retryable")
	}

	// Unknown: be conservative and allow retry.
	be = &domain.BootError{Kind: domain.BootErrUnknown}
	if !be.Retryable() {
		t.Error("unknown should be retryable (conservative)")
	}

	// Nil: not retryable (nothing to retry).
	var nilBe *domain.BootError
	if nilBe.Retryable() {
		t.Error("nil BootError should not be retryable")
	}
}
