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

	getPublicKeyFn    func(*vaultpb.GetPublicKeyRequest) (*vaultpb.GetPublicKeyResponse, error)
	decryptScoresFn   func(*vaultpb.DecryptScoresRequest) (*vaultpb.DecryptScoresResponse, error)
	decryptMetadataFn func(*vaultpb.DecryptMetadataRequest) (*vaultpb.DecryptMetadataResponse, error)
	healthFn          func(*healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error)
}

func (f *fakeServer) GetPublicKey(_ context.Context, req *vaultpb.GetPublicKeyRequest) (*vaultpb.GetPublicKeyResponse, error) {
	if f.getPublicKeyFn != nil {
		return f.getPublicKeyFn(req)
	}
	return nil, status.Error(codes.Unimplemented, "test server: GetPublicKey not stubbed")
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

// validBundleJSON returns a JSON string matching Vault's buildBundle output
// (rune-admin/vault/internal/server/grpc.go:131-153).
func validBundleJSON(t *testing.T) string {
	t.Helper()
	dek := make([]byte, 32)
	for i := range dek {
		dek[i] = byte(i)
	}
	return `{
		"EncKey.json": "<enc-bytes>",
		"EvalKey.json": "<eval-bytes>",
		"key_id": "key_test",
		"index_name": "test-index",
		"agent_id": "agent_test",
		"agent_dek": "` + base64.StdEncoding.EncodeToString(dek) + `",
		"envector_endpoint": "https://envector.test",
		"envector_api_key": "env_apikey"
	}`
}

func TestGetPublicKey_HappyPath(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.getPublicKeyFn = func(req *vaultpb.GetPublicKeyRequest) (*vaultpb.GetPublicKeyResponse, error) {
		if req.GetToken() != "test-token" {
			return nil, status.Error(codes.Unauthenticated, "wrong token")
		}
		return &vaultpb.GetPublicKeyResponse{KeyBundleJson: validBundleJSON(t)}, nil
	}

	bundle, err := c.GetPublicKey(context.Background())
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
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

func TestGetPublicKey_ResponseErrorString(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.getPublicKeyFn = func(*vaultpb.GetPublicKeyRequest) (*vaultpb.GetPublicKeyResponse, error) {
		return &vaultpb.GetPublicKeyResponse{Error: "bundle build failed"}, nil
	}

	_, err := c.GetPublicKey(context.Background())
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
	if !strings.Contains(ve.Message, "bundle build failed") {
		t.Errorf("Message: got %q, want substring 'bundle build failed'", ve.Message)
	}
}

func TestGetPublicKey_BadDEKLength(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.getPublicKeyFn = func(*vaultpb.GetPublicKeyRequest) (*vaultpb.GetPublicKeyResponse, error) {
		// 16 bytes — wrong (must be 32 for AES-256)
		dek := base64.StdEncoding.EncodeToString(make([]byte, 16))
		return &vaultpb.GetPublicKeyResponse{
			KeyBundleJson: `{"agent_dek": "` + dek + `"}`,
		}, nil
	}

	_, err := c.GetPublicKey(context.Background())
	if err == nil {
		t.Fatal("expected error for bad DEK length, got nil")
	}
	if !strings.Contains(err.Error(), "agent_dek size 16") {
		t.Errorf("error message: got %q, want substring 'agent_dek size 16'", err.Error())
	}
}

func TestGetPublicKey_MalformedJSON(t *testing.T) {
	fake, c := startFakeServer(t)
	fake.getPublicKeyFn = func(*vaultpb.GetPublicKeyRequest) (*vaultpb.GetPublicKeyResponse, error) {
		return &vaultpb.GetPublicKeyResponse{KeyBundleJson: "not json {"}, nil
	}

	_, err := c.GetPublicKey(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parse key bundle json") {
		t.Errorf("error: got %q, want 'parse key bundle json' substring", err.Error())
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

// MapGRPCError code matrix — verifies the spec-mandated mapping table.
func TestMapGRPCError_CodeMatrix(t *testing.T) {
	cases := []struct {
		name      string
		grpcCode  codes.Code
		wantCode  string
		retryable bool
	}{
		{"Unauthenticated → AUTH_FAILED", codes.Unauthenticated, "VAULT_AUTH_FAILED", false},
		{"NotFound → KEY_NOT_FOUND", codes.NotFound, "VAULT_KEY_NOT_FOUND", false},
		{"Unavailable → UNAVAILABLE", codes.Unavailable, "VAULT_UNAVAILABLE", true},
		{"DeadlineExceeded → TIMEOUT", codes.DeadlineExceeded, "VAULT_TIMEOUT", true},
		{"Internal → INTERNAL", codes.Internal, "VAULT_INTERNAL", true},
		{"PermissionDenied → INTERNAL (default)", codes.PermissionDenied, "VAULT_INTERNAL", true},
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

func TestValidateAgentDEK(t *testing.T) {
	cases := []struct {
		name string
		size int
		want bool // true == should pass
	}{
		{"32B passes", 32, true},
		{"16B fails (AES-128 size)", 16, false},
		{"64B fails", 64, false},
		{"0B fails", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := vault.ValidateAgentDEK(make([]byte, tc.size))
			gotPass := err == nil
			if gotPass != tc.want {
				t.Errorf("size %d: pass=%v, want %v (err=%v)", tc.size, gotPass, tc.want, err)
			}
		})
	}
}

// _ = time keeps the import warm if other tests are deleted.
var _ = time.Second
