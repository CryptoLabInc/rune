package vault_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	vaultpb "github.com/CryptoLabInc/rune-admin/vault/pkg/vaultpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/envector/rune-go/internal/adapters/vault"
)

// fakeServer implements VaultServiceServer + HealthServer for in-process tests.
// All responses are programmable per-call via the corresponding func fields.
type fakeServer struct {
	vaultpb.UnimplementedVaultServiceServer
	healthpb.UnimplementedHealthServer

	getAgentManifestFn func(*vaultpb.GetAgentManifestRequest) (*vaultpb.GetAgentManifestResponse, error)
	decryptScoresFn    func(*vaultpb.DecryptScoresRequest) (*vaultpb.DecryptScoresResponse, error)
	decryptMetadataFn  func(*vaultpb.DecryptMetadataRequest) (*vaultpb.DecryptMetadataResponse, error)
	healthFn           func(*healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error)
}

func (f *fakeServer) GetAgentManifest(_ context.Context, req *vaultpb.GetAgentManifestRequest) (*vaultpb.GetAgentManifestResponse, error) {
	if f.getAgentManifestFn != nil {
		return f.getAgentManifestFn(req)
	}
	return nil, status.Error(codes.Unimplemented, "test server: GetAgentManifest not stubbed")
}

func (f *fakeServer) DecryptScores(_ context.Context, req *vaultpb.DecryptScoresRequest) (*vaultpb.DecryptScoresResponse, error) {
	if f.decryptScoresFn != nil {
		return f.decryptScoresFn(req)
	}
	return nil, status.Error(codes.Unimplemented, "test server: DecryptScores not stubbed")
}

func (f *fakeServer) DecryptMetadata(_ context.Context, req *vaultpb.DecryptMetadataRequest) (*vaultpb.DecryptMetadataResponse, error) {
	if f.decryptMetadataFn != nil {
		return f.decryptMetadataFn(req)
	}
	return nil, status.Error(codes.Unimplemented, "test server: DecryptMetadata not stubbed")
}

func (f *fakeServer) Check(_ context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	if f.healthFn != nil {
		return f.healthFn(req)
	}
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

// startFakeServer spins up a bufconn-backed server and returns a connected
// vault.Client + cleanup function. Tests modify fake.* func fields between
// dial and call to control responses.
func startFakeServer(t *testing.T) (*fakeServer, vault.Client) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	fake := &fakeServer{}
	vaultpb.RegisterVaultServiceServer(srv, fake)
	healthpb.RegisterHealthServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(
		"passthrough://bufconn",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(context.Background())
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	c := vault.NewBufconnClient(conn, "test-token")
	return fake, c
}

// validManifestJSON returns a JSON string matching Vault's GetAgentManifest
// payload shape (rune-admin/vault/internal/server/grpc.go buildBundle).
// Note: post-#103 the manifest no longer carries EvalKey.json.
func validManifestJSON(t *testing.T) string {
	t.Helper()
	dek := make([]byte, 32)
	for i := range dek {
		dek[i] = byte(i)
	}
	return `{
		"EncKey.json": "<enc-bytes>",
		"key_id": "key_test",
		"index_name": "test-index",
		"agent_id": "agent_test",
		"agent_dek": "` + base64.StdEncoding.EncodeToString(dek) + `",
		"envector_endpoint": "https://envector.test",
		"envector_api_key": "env_apikey"
	}`
}

func TestGetAgentManifest_HappyPath(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.getAgentManifestFn = func(req *vaultpb.GetAgentManifestRequest) (*vaultpb.GetAgentManifestResponse, error) {
		if req.GetToken() != "test-token" {
			return nil, status.Error(codes.Unauthenticated, "wrong token")
		}
		return &vaultpb.GetAgentManifestResponse{ManifestJson: validManifestJSON(t)}, nil
	}

	bundle, err := c.GetAgentManifest(context.Background())
	if err != nil {
		t.Fatalf("GetAgentManifest: %v", err)
	}
	if bundle.KeyID != "key_test" {
		t.Errorf("KeyID: got %q, want key_test", bundle.KeyID)
	}
	if bundle.IndexName != "test-index" {
		t.Errorf("IndexName: got %q, want test-index", bundle.IndexName)
	}
	if bundle.AgentID != "agent_test" {
		t.Errorf("AgentID: got %q, want agent_test", bundle.AgentID)
	}
	if len(bundle.AgentDEK) != 32 {
		t.Errorf("AgentDEK length: got %d, want 32", len(bundle.AgentDEK))
	}
	if bundle.EnvectorEndpoint != "https://envector.test" {
		t.Errorf("EnvectorEndpoint: got %q", bundle.EnvectorEndpoint)
	}
}

func TestGetAgentManifest_ResponseErrorString(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.getAgentManifestFn = func(*vaultpb.GetAgentManifestRequest) (*vaultpb.GetAgentManifestResponse, error) {
		return &vaultpb.GetAgentManifestResponse{Error: "manifest build failed"}, nil
	}

	_, err := c.GetAgentManifest(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ve *vault.Error
	if !errors.As(err, &ve) {
		t.Fatalf("expected *vault.Error, got %T: %v", err, err)
	}
	if ve.Code != "VAULT_INTERNAL" {
		t.Errorf("Code: got %q, want VAULT_INTERNAL", ve.Code)
	}
	if !strings.Contains(ve.Message, "manifest build failed") {
		t.Errorf("Message: got %q, want substring 'manifest build failed'", ve.Message)
	}
}

func TestGetAgentManifest_BadDEKLength(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.getAgentManifestFn = func(*vaultpb.GetAgentManifestRequest) (*vaultpb.GetAgentManifestResponse, error) {
		// 16 bytes — wrong (must be 32 for AES-256)
		dek := base64.StdEncoding.EncodeToString(make([]byte, 16))
		return &vaultpb.GetAgentManifestResponse{
			ManifestJson: `{"agent_dek": "` + dek + `"}`,
		}, nil
	}

	_, err := c.GetAgentManifest(context.Background())
	if err == nil {
		t.Fatal("expected error for bad DEK length, got nil")
	}
	if !strings.Contains(err.Error(), "agent_dek size 16") {
		t.Errorf("error message: got %q, want substring 'agent_dek size 16'", err.Error())
	}
}

func TestGetAgentManifest_MalformedJSON(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.getAgentManifestFn = func(*vaultpb.GetAgentManifestRequest) (*vaultpb.GetAgentManifestResponse, error) {
		return &vaultpb.GetAgentManifestResponse{ManifestJson: "not json {"}, nil
	}

	_, err := c.GetAgentManifest(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parse manifest_json") {
		t.Errorf("error: got %q, want 'parse manifest_json' substring", err.Error())
	}
}

func TestParseManifestJSON_DropsEvalKeyField(t *testing.T) {
	// EvalKey.json field, if present (legacy manifest), is silently ignored — the
	// new contract drops it from the plugin-side Bundle. Verifies forward compat
	// with stale Vault responses that still include EvalKey.json.
	rawWithEval := `{
		"EncKey.json": "enc",
		"EvalKey.json": "eval-should-be-ignored",
		"key_id": "k",
		"agent_id": "a",
		"agent_dek": "` + base64.StdEncoding.EncodeToString(make([]byte, 32)) + `",
		"envector_endpoint": "https://e",
		"envector_api_key": "k",
		"index_name": "i"
	}`
	bundle, err := vault.ParseManifestJSON(rawWithEval)
	if err != nil {
		t.Fatalf("ParseManifestJSON: %v", err)
	}
	// Bundle struct doesn't have an EvalKey field — extra JSON key is ignored.
	if string(bundle.EncKey) != "enc" {
		t.Errorf("EncKey: got %q, want enc", string(bundle.EncKey))
	}
}

func TestDecryptScores_HappyPath(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.decryptScoresFn = func(req *vaultpb.DecryptScoresRequest) (*vaultpb.DecryptScoresResponse, error) {
		if req.GetTopK() != 5 {
			t.Errorf("top_k: got %d, want 5", req.GetTopK())
		}
		if req.GetEncryptedBlobB64() != "blob123" {
			t.Errorf("blob: got %q", req.GetEncryptedBlobB64())
		}
		return &vaultpb.DecryptScoresResponse{
			Results: []*vaultpb.ScoreEntry{
				{ShardIdx: 0, RowIdx: 1, Score: 0.95},
				{ShardIdx: 0, RowIdx: 2, Score: 0.80},
			},
		}, nil
	}

	out, err := c.DecryptScores(context.Background(), "blob123", 5)
	if err != nil {
		t.Fatalf("DecryptScores: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("results: got %d, want 2", len(out))
	}
	if out[0].Score != 0.95 || out[0].RowIdx != 1 {
		t.Errorf("results[0]: got %+v", out[0])
	}
}

func TestDecryptMetadata_HappyPath(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.decryptMetadataFn = func(req *vaultpb.DecryptMetadataRequest) (*vaultpb.DecryptMetadataResponse, error) {
		if len(req.GetEncryptedMetadataList()) != 2 {
			t.Errorf("list len: got %d, want 2", len(req.GetEncryptedMetadataList()))
		}
		return &vaultpb.DecryptMetadataResponse{
			DecryptedMetadata: []string{`{"a":1}`, `{"b":2}`},
		}, nil
	}

	out, err := c.DecryptMetadata(context.Background(), []string{"env1", "env2"})
	if err != nil {
		t.Fatalf("DecryptMetadata: %v", err)
	}
	if len(out) != 2 || out[0] != `{"a":1}` || out[1] != `{"b":2}` {
		t.Errorf("decrypted: got %v", out)
	}
}

func TestHealthCheck_Serving(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.healthFn = func(*healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
		return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
	}

	healthy, err := c.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if !healthy {
		t.Error("expected healthy=true")
	}
}

func TestHealthCheck_NotServing(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.healthFn = func(*healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
		return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_NOT_SERVING}, nil
	}

	healthy, err := c.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if healthy {
		t.Error("expected healthy=false")
	}
}

// MapGRPCError code matrix — verifies every code rune-admin/vault server
// emits (PermissionDenied, InvalidArgument, ResourceExhausted, Unauthenticated,
// Internal) plus the transport-layer codes (Unavailable, DeadlineExceeded)
// and the legacy NotFound mapping. Retryable flags follow the audit:
// permission / input failures are non-retryable; transport / rate / internal
// are retryable.
func TestMapGRPCError_CodeMatrix(t *testing.T) {
	cases := []struct {
		name      string
		grpcCode  codes.Code
		wantCode  string
		retryable bool
	}{
		{"Unauthenticated → AUTH_FAILED", codes.Unauthenticated, "VAULT_AUTH_FAILED", false},
		{"PermissionDenied → PERMISSION_DENIED", codes.PermissionDenied, "VAULT_PERMISSION_DENIED", false},
		{"InvalidArgument → INVALID_INPUT", codes.InvalidArgument, "VAULT_INVALID_INPUT", false},
		{"ResourceExhausted → RATE_LIMITED", codes.ResourceExhausted, "VAULT_RATE_LIMITED", true},
		{"NotFound → KEY_NOT_FOUND", codes.NotFound, "VAULT_KEY_NOT_FOUND", false},
		{"Unavailable → UNAVAILABLE", codes.Unavailable, "VAULT_UNAVAILABLE", true},
		{"DeadlineExceeded → TIMEOUT", codes.DeadlineExceeded, "VAULT_TIMEOUT", true},
		{"Internal → INTERNAL", codes.Internal, "VAULT_INTERNAL", true},
		{"Aborted → INTERNAL (default)", codes.Aborted, "VAULT_INTERNAL", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := vault.MapGRPCError(status.Error(tc.grpcCode, "test message"))
			var ve *vault.Error
			if !errors.As(err, &ve) {
				t.Fatalf("expected *vault.Error, got %T", err)
			}
			if ve.Code != tc.wantCode {
				t.Errorf("Code: got %q, want %q", ve.Code, tc.wantCode)
			}
			if ve.Retryable != tc.retryable {
				t.Errorf("Retryable: got %v, want %v", ve.Retryable, tc.retryable)
			}
		})
	}
}

func TestMapGRPCError_NonGRPCFallback(t *testing.T) {
	err := vault.MapGRPCError(errors.New("plain error"))
	var ve *vault.Error
	if !errors.As(err, &ve) {
		t.Fatalf("expected *vault.Error, got %T", err)
	}
	if ve.Code != "VAULT_INTERNAL" {
		t.Errorf("non-gRPC error: got Code %q, want VAULT_INTERNAL", ve.Code)
	}
}

func TestMapGRPCError_NilReturnsNil(t *testing.T) {
	if got := vault.MapGRPCError(nil); got != nil {
		t.Errorf("MapGRPCError(nil): got %v, want nil", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Edge-case coverage for decodeAgentDEK paths via GetAgentManifest.
// ─────────────────────────────────────────────────────────────────────────────

func TestGetAgentManifest_EmptyDEK(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.getAgentManifestFn = func(*vaultpb.GetAgentManifestRequest) (*vaultpb.GetAgentManifestResponse, error) {
		// agent_dek empty string → fast-path "agent_dek is empty"
		return &vaultpb.GetAgentManifestResponse{
			ManifestJson: `{"agent_dek": ""}`,
		}, nil
	}

	_, err := c.GetAgentManifest(context.Background())
	if err == nil {
		t.Fatal("expected error for empty agent_dek, got nil")
	}
	if !strings.Contains(err.Error(), "agent_dek is empty") {
		t.Errorf("error: got %q, want substring 'agent_dek is empty'", err.Error())
	}
}

func TestGetAgentManifest_InvalidBase64DEK(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.getAgentManifestFn = func(*vaultpb.GetAgentManifestRequest) (*vaultpb.GetAgentManifestResponse, error) {
		// invalid base64 chars
		return &vaultpb.GetAgentManifestResponse{
			ManifestJson: `{"agent_dek": "!!!@@@$$$"}`,
		}, nil
	}

	_, err := c.GetAgentManifest(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid base64 agent_dek, got nil")
	}
	if !strings.Contains(err.Error(), "invalid base64") {
		t.Errorf("error: got %q, want substring 'invalid base64'", err.Error())
	}
}

func TestGetAgentManifest_DEKLengthMatrix(t *testing.T) {
	cases := []struct {
		name      string
		dekLen    int
		shouldErr bool
	}{
		{"0 bytes (would hit empty path if base64 round-trips to '')", 0, true}, // base64("") = "" → empty fast path
		{"16 bytes (AES-128)", 16, true},
		{"31 bytes (off-by-one)", 31, true},
		{"32 bytes (AES-256, valid)", 32, false},
		{"33 bytes (off-by-one)", 33, true},
		{"64 bytes (AES-512, mistakenly used)", 64, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake, c := startFakeServer(t)
			fake.getAgentManifestFn = func(*vaultpb.GetAgentManifestRequest) (*vaultpb.GetAgentManifestResponse, error) {
				dek := base64.StdEncoding.EncodeToString(make([]byte, tc.dekLen))
				return &vaultpb.GetAgentManifestResponse{
					ManifestJson: `{"agent_dek": "` + dek + `"}`,
				}, nil
			}

			_, err := c.GetAgentManifest(context.Background())
			if tc.shouldErr && err == nil {
				t.Fatalf("expected error for DEK length %d", tc.dekLen)
			}
			if !tc.shouldErr && err != nil {
				t.Fatalf("unexpected error for DEK length %d: %v", tc.dekLen, err)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RPC integration → MapGRPCError → typed *Error end-to-end coverage.
// Server emits gRPC status; client must surface a typed *vault.Error with
// correct Code/Retryable.
// ─────────────────────────────────────────────────────────────────────────────

func TestGetAgentManifest_GRPCErrorMatrix(t *testing.T) {
	cases := []struct {
		name      string
		grpcCode  codes.Code
		wantCode  string
		retryable bool
	}{
		{"Unauthenticated", codes.Unauthenticated, "VAULT_AUTH_FAILED", false},
		{"PermissionDenied", codes.PermissionDenied, "VAULT_PERMISSION_DENIED", false},
		{"InvalidArgument", codes.InvalidArgument, "VAULT_INVALID_INPUT", false},
		{"ResourceExhausted", codes.ResourceExhausted, "VAULT_RATE_LIMITED", true},
		{"Internal", codes.Internal, "VAULT_INTERNAL", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake, c := startFakeServer(t)
			fake.getAgentManifestFn = func(*vaultpb.GetAgentManifestRequest) (*vaultpb.GetAgentManifestResponse, error) {
				return nil, status.Error(tc.grpcCode, "server says "+tc.name)
			}

			_, err := c.GetAgentManifest(context.Background())
			if err == nil {
				t.Fatal("expected error from gRPC status")
			}
			var ve *vault.Error
			if !errors.As(err, &ve) {
				t.Fatalf("expected *vault.Error, got %T: %v", err, err)
			}
			if ve.Code != tc.wantCode {
				t.Errorf("Code: got %q, want %q", ve.Code, tc.wantCode)
			}
			if ve.Retryable != tc.retryable {
				t.Errorf("Retryable: got %v, want %v", ve.Retryable, tc.retryable)
			}
		})
	}
}

func TestDecryptScores_GRPCError(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.decryptScoresFn = func(*vaultpb.DecryptScoresRequest) (*vaultpb.DecryptScoresResponse, error) {
		return nil, status.Error(codes.PermissionDenied, "topk exceeds role limit")
	}

	_, err := c.DecryptScores(context.Background(), "blob", 100)
	if err == nil {
		t.Fatal("expected error")
	}
	var ve *vault.Error
	if !errors.As(err, &ve) {
		t.Fatalf("expected *vault.Error, got %T", err)
	}
	if ve.Code != "VAULT_PERMISSION_DENIED" {
		t.Errorf("Code: got %q, want VAULT_PERMISSION_DENIED", ve.Code)
	}
	if ve.Retryable {
		t.Error("PermissionDenied must not be retryable")
	}
}

func TestDecryptMetadata_GRPCError(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.decryptMetadataFn = func(*vaultpb.DecryptMetadataRequest) (*vaultpb.DecryptMetadataResponse, error) {
		return nil, status.Error(codes.InvalidArgument, "list size exceeds 1000")
	}

	_, err := c.DecryptMetadata(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("expected error")
	}
	var ve *vault.Error
	if !errors.As(err, &ve) {
		t.Fatalf("expected *vault.Error, got %T", err)
	}
	if ve.Code != "VAULT_INVALID_INPUT" {
		t.Errorf("Code: got %q, want VAULT_INVALID_INPUT", ve.Code)
	}
	if ve.Retryable {
		t.Error("InvalidArgument must not be retryable")
	}
}

func TestHealthCheck_GRPCError(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.healthFn = func(*healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
		return nil, status.Error(codes.Unavailable, "server down")
	}

	healthy, err := c.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error from gRPC status")
	}
	if healthy {
		t.Error("expected healthy=false on error")
	}
	var ve *vault.Error
	if !errors.As(err, &ve) {
		t.Fatalf("expected *vault.Error, got %T", err)
	}
	if ve.Code != "VAULT_UNAVAILABLE" {
		t.Errorf("Code: got %q, want VAULT_UNAVAILABLE", ve.Code)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MapGRPCError invariants — Cause preservation + errors.As chain + Message.
// ─────────────────────────────────────────────────────────────────────────────

func TestMapGRPCError_PreservesCause(t *testing.T) {
	allCodes := []codes.Code{
		codes.Unauthenticated, codes.PermissionDenied, codes.InvalidArgument,
		codes.ResourceExhausted, codes.NotFound, codes.Unavailable,
		codes.DeadlineExceeded, codes.Internal, codes.Aborted, // last = default branch
	}
	for _, code := range allCodes {
		t.Run(code.String(), func(t *testing.T) {
			origin := status.Error(code, "x")
			err := vault.MapGRPCError(origin)
			var ve *vault.Error
			if !errors.As(err, &ve) {
				t.Fatalf("expected *vault.Error, got %T", err)
			}
			if ve.Cause != origin {
				t.Errorf("Cause: got %v, want original gRPC err", ve.Cause)
			}
			// errors.Is should walk Unwrap → find the original gRPC status err
			if !errors.Is(err, origin) {
				t.Error("errors.Is(*Error, originalErr) should be true via Unwrap")
			}
		})
	}
}

func TestMapGRPCError_PreservesMessage(t *testing.T) {
	msg := "very specific server message"
	err := vault.MapGRPCError(status.Error(codes.PermissionDenied, msg))
	var ve *vault.Error
	if !errors.As(err, &ve) {
		t.Fatalf("expected *vault.Error, got %T", err)
	}
	if ve.Message != msg {
		t.Errorf("Message: got %q, want %q", ve.Message, msg)
	}
}

func TestError_ErrorString(t *testing.T) {
	cases := []struct {
		ve   *vault.Error
		want string
	}{
		{&vault.Error{Code: "X"}, "X"},
		{&vault.Error{Code: "X", Message: "y"}, "X: y"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.ve.Error(); got != tc.want {
				t.Errorf("Error(): got %q, want %q", got, tc.want)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Bundle / response-error / boundary-input coverage.
// ─────────────────────────────────────────────────────────────────────────────

func TestGetAgentManifest_FullBundleEquality(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.getAgentManifestFn = func(*vaultpb.GetAgentManifestRequest) (*vaultpb.GetAgentManifestResponse, error) {
		return &vaultpb.GetAgentManifestResponse{ManifestJson: validManifestJSON(t)}, nil
	}

	bundle, err := c.GetAgentManifest(context.Background())
	if err != nil {
		t.Fatalf("GetAgentManifest: %v", err)
	}
	// The fields not asserted in TestGetAgentManifest_HappyPath:
	if string(bundle.EncKey) != "<enc-bytes>" {
		t.Errorf("EncKey: got %q, want '<enc-bytes>'", string(bundle.EncKey))
	}
	if bundle.EnvectorAPIKey != "env_apikey" {
		t.Errorf("EnvectorAPIKey: got %q", bundle.EnvectorAPIKey)
	}
}

func TestGetAgentManifest_OmittedIndexName(t *testing.T) {
	// Server omits index_name when not configured (buildBundle:140-142).
	fake, c := startFakeServer(t)
	fake.getAgentManifestFn = func(*vaultpb.GetAgentManifestRequest) (*vaultpb.GetAgentManifestResponse, error) {
		dek := base64.StdEncoding.EncodeToString(make([]byte, 32))
		return &vaultpb.GetAgentManifestResponse{
			ManifestJson: `{"EncKey.json":"e","key_id":"k","agent_id":"a","agent_dek":"` + dek + `","envector_endpoint":"x","envector_api_key":"y"}`,
		}, nil
	}
	bundle, err := c.GetAgentManifest(context.Background())
	if err != nil {
		t.Fatalf("GetAgentManifest: %v", err)
	}
	if bundle.IndexName != "" {
		t.Errorf("IndexName when omitted: got %q, want empty string", bundle.IndexName)
	}
}

func TestDecryptScores_ResponseErrorString(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.decryptScoresFn = func(*vaultpb.DecryptScoresRequest) (*vaultpb.DecryptScoresResponse, error) {
		return &vaultpb.DecryptScoresResponse{Error: "fhe decryption failed"}, nil
	}
	_, err := c.DecryptScores(context.Background(), "x", 5)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "fhe decryption failed") {
		t.Errorf("error msg: got %q", err.Error())
	}
}

func TestDecryptMetadata_ResponseErrorString(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.decryptMetadataFn = func(*vaultpb.DecryptMetadataRequest) (*vaultpb.DecryptMetadataResponse, error) {
		return &vaultpb.DecryptMetadataResponse{Error: "envelope corrupted"}, nil
	}
	_, err := c.DecryptMetadata(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "envelope corrupted") {
		t.Errorf("error msg: got %q", err.Error())
	}
}

func TestDecryptScores_EmptyResults(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.decryptScoresFn = func(*vaultpb.DecryptScoresRequest) (*vaultpb.DecryptScoresResponse, error) {
		return &vaultpb.DecryptScoresResponse{Results: nil}, nil
	}
	out, err := c.DecryptScores(context.Background(), "x", 5)
	if err != nil {
		t.Fatalf("DecryptScores: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("results: got %d, want 0", len(out))
	}
}

func TestDecryptMetadata_EmptyList(t *testing.T) {
	fake, c := startFakeServer(t)
	called := false
	fake.decryptMetadataFn = func(req *vaultpb.DecryptMetadataRequest) (*vaultpb.DecryptMetadataResponse, error) {
		called = true
		if len(req.GetEncryptedMetadataList()) != 0 {
			t.Errorf("server got list of %d, want 0", len(req.GetEncryptedMetadataList()))
		}
		return &vaultpb.DecryptMetadataResponse{DecryptedMetadata: nil}, nil
	}
	out, err := c.DecryptMetadata(context.Background(), nil)
	if err != nil {
		t.Fatalf("DecryptMetadata: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("got %d, want 0", len(out))
	}
	if !called {
		t.Error("server should have been called even for empty list (current behavior)")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ParseManifestJSON direct cases — required-field absence + structurally bad
// inputs. (forward-compat case is in TestParseManifestJSON_DropsEvalKeyField.)
// ─────────────────────────────────────────────────────────────────────────────

func TestParseManifestJSON_EmptyJSON(t *testing.T) {
	_, err := vault.ParseManifestJSON(`{}`)
	if err == nil {
		t.Fatal("expected error for empty JSON (missing agent_dek)")
	}
	if !strings.Contains(err.Error(), "agent_dek is empty") {
		t.Errorf("error: got %q, want 'agent_dek is empty'", err.Error())
	}
}

func TestParseManifestJSON_MissingAgentDEK(t *testing.T) {
	raw := `{"EncKey.json":"e","key_id":"k","agent_id":"a","envector_endpoint":"x","envector_api_key":"y"}`
	_, err := vault.ParseManifestJSON(raw)
	if err == nil {
		t.Fatal("expected error for missing agent_dek")
	}
}

func TestParseManifestJSON_NotJSON(t *testing.T) {
	_, err := vault.ParseManifestJSON("not json at all")
	if err == nil {
		t.Fatal("expected JSON parse error")
	}
	if !strings.Contains(err.Error(), "parse manifest_json") {
		t.Errorf("error: got %q", err.Error())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Lifecycle / boundary — Endpoint + Close + ctx cancellation.
// ─────────────────────────────────────────────────────────────────────────────

func TestEndpoint_ReturnsConstructorValue(t *testing.T) {
	_, c := startFakeServer(t)
	if c.Endpoint() != "bufconn" {
		t.Errorf("Endpoint(): got %q, want bufconn", c.Endpoint())
	}
}

func TestClose_NilSafe(t *testing.T) {
	_, c := startFakeServer(t)
	if err := c.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	// Calling Close twice — second call should not panic. grpc.ClientConn.Close
	// returns a "connection is closing" error after first call; we tolerate
	// either nil or non-nil but no panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("second Close panicked: %v", r)
		}
	}()
	_ = c.Close()
}

func TestGetAgentManifest_ContextCanceled(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.getAgentManifestFn = func(*vaultpb.GetAgentManifestRequest) (*vaultpb.GetAgentManifestResponse, error) {
		// server intentionally hangs — simulate slow/unresponsive Vault.
		time.Sleep(2 * time.Second)
		return &vaultpb.GetAgentManifestResponse{ManifestJson: validManifestJSON(t)}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, err := c.GetAgentManifest(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled ctx")
	}
	// Cancelled context surfaces as gRPC Canceled status → MapGRPCError default → VAULT_INTERNAL
	// (or Canceled-specific behavior depending on grpc-go runtime).
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") &&
		!strings.Contains(err.Error(), "Canceled") && !strings.Contains(err.Error(), "VAULT_INTERNAL") {
		t.Errorf("expected ctx cancel signal in error, got %v", err)
	}
}

// _ = time keeps the import warm if other tests are deleted.
var _ = time.Second
