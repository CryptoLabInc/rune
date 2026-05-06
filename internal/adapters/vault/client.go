// Package vault is the Rune-Vault gRPC client.
// Spec: docs/v04/spec/components/vault.md.
// Python: mcp/adapter/vault_client.py (381 LoC).
//
// Responsibility:
//   - GetAgentManifest: fetch agent manifest (EncKey, envector creds, agent_dek)
//   - DecryptScores: Vault decrypts encrypted_blob → [{shard, row, score}]
//   - DecryptMetadata: Vault decrypts AES envelopes → plaintext JSON strings
//
// Key ownership:
//   - Vault owns EvalKey and SecKey to handle RegisterKeys/LoadKeys/decryption
//   - Plugin receives EncKey + agent_dek only via GetAgentManifest
//
// Asymmetric responsibility (critical):
//   - Capture: rune-mcp service layer encrypts locally with agent_dek
//   - Recall: service layer calls DecryptMetadata (Python searcher.py:L444,L455)
//     envector SDK is NEVER in the decrypt path
package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	vaultpb "github.com/CryptoLabInc/rune-admin/vault/pkg/vaultpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

// Applied on both MaxCallRecvMsgSize and MaxCallSendMsgSize
const MaxMessageLength = 16 * 1024 * 1024

// DefaultTimeout — Python vault_client.py:L84 (all RPCs: 30s)
const DefaultTimeout = 30 * time.Second

// HealthCheckTimeout — Python vault_client.py:L315 (5s override on health probes)
const HealthCheckTimeout = 5 * time.Second

// Returned by GetAgentManifest (GetAgentManifestResponse.manifest_json).
type Bundle struct {
	EncKey           []byte // FHE encryption key (for local encrypt via envector SDK)
	EnvectorEndpoint string // enVector Cloud server address
	EnvectorAPIKey   string // enVector Cloud access token
	AgentID          string // agent identifier (used in AES envelope "a" field)
	AgentDEK         []byte // MUST be exactly 32 bytes (AES-256)
	KeyID            string // key bundle identifier
	IndexName        string // server-side index name
}

type manifestJSON struct {
	EncKeyJSON       string `json:"EncKey.json"`
	EnvectorEndpoint string `json:"envector_endpoint"`
	EnvectorAPIKey   string `json:"envector_api_key"`
	AgentID          string `json:"agent_id"`
	AgentDEK         string `json:"agent_dek"` // base64(StdEncoding)
	KeyID            string `json:"key_id"`
	IndexName        string `json:"index_name"`
}

func ParseManifestJSON(raw string) (*Bundle, error) {
	var m manifestJSON
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("vault: parse manifest_json: %w", err)
	}

	dek, err := decodeAgentDEK(m.AgentDEK)
	if err != nil {
		return nil, err
	}

	return &Bundle{
		EncKey:           []byte(m.EncKeyJSON),
		EnvectorEndpoint: m.EnvectorEndpoint,
		EnvectorAPIKey:   m.EnvectorAPIKey,
		AgentID:          m.AgentID,
		AgentDEK:         dek,
		KeyID:            m.KeyID,
		IndexName:        m.IndexName,
	}, nil
}

// base64 decode + 32-byte length check
// length mismatch: non-retryable
func decodeAgentDEK(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, fmt.Errorf("vault: agent_dek is empty")
	}

	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("vault: agent_dek invalid base64: %w", err)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("vault: invalid agent_dek size %d (expected 32)", len(b))
	}

	return b, nil
}

// ScoreEntry — DecryptScores output.
type ScoreEntry struct {
	ShardIdx int32
	RowIdx   int32
	Score    float64
}

// Client interface — implemented by gRPC client (and test mocks).
type Client interface {
	GetAgentManifest(ctx context.Context) (*Bundle, error)
	DecryptScores(ctx context.Context, encryptedBlobB64 string, topK int) ([]ScoreEntry, error)
	DecryptMetadata(ctx context.Context, encryptedMetadataList []string) ([]string, error)
	HealthCheck(ctx context.Context) (bool, error)
	Endpoint() string
	Close() error
}

type ClientOpts struct {
	CACertPath string // path to PEM; empty = system CA bundle
	TLSDisable bool
}

// client is the gRPC implementation.
type client struct {
	endpoint string
	token    string
	conn     *grpc.ClientConn
	stub     vaultpb.VaultServiceClient
}

var defaultKeepalive = keepalive.ClientParameters{
	Time:                30 * time.Second,
	Timeout:             10 * time.Second,
	PermitWithoutStream: true,
}

// See spec/components/vault.md §TLS + §Keepalive.
func NewClient(endpoint, token string, opts ClientOpts) (Client, error) {
	normalized, err := NormalizeEndpoint(endpoint)
	if err != nil {
		return nil, fmt.Errorf("vault: invalid endpoint: %w", err)
	}

	dialOpts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(MaxMessageLength),
			grpc.MaxCallSendMsgSize(MaxMessageLength),
		),
		grpc.WithKeepaliveParams(defaultKeepalive),
	}

	switch {
	case opts.TLSDisable:
		slog.Warn("vault: TLS disabled — gRPC traffic is unencrypted. Only use for local development.")
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	case opts.CACertPath != "":
		creds, err := credentials.NewClientTLSFromFile(opts.CACertPath, "")
		if err != nil {
			return nil, fmt.Errorf("vault: failed to load CA cert %s: %w", opts.CACertPath, err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	default:
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(nil)))
	}

	conn, err := grpc.NewClient(normalized, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("vault: grpc dial failed: %w", err)
	}

	slog.Info("vault: connected", "endpoint", normalized)
	return &client{
		endpoint: normalized,
		token:    token,
		conn:     conn,
		stub:     vaultpb.NewVaultServiceClient(conn),
	}, nil
}

func (c *client) authCtx(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.token)
}

func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) <= d {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

func (c *client) GetAgentManifest(ctx context.Context) (*Bundle, error) {
	ctx, cancel := withTimeout(c.authCtx(ctx), DefaultTimeout)
	defer cancel()

	resp, err := c.stub.GetAgentManifest(ctx, &vaultpb.GetAgentManifestRequest{Token: c.token})
	if err != nil {
		return nil, MapGRPCError(err)
	}
	if msg := resp.GetError(); msg != "" {
		return nil, &Error{Code: ErrVaultInternal.Code, Message: "GetAgentManifest: " + msg, Retryable: true}
	}

	return ParseManifestJSON(resp.GetManifestJson())
}

func (c *client) DecryptScores(ctx context.Context, encryptedBlobB64 string, topK int) ([]ScoreEntry, error) {
	ctx, cancel := withTimeout(c.authCtx(ctx), DefaultTimeout)
	defer cancel()

	resp, err := c.stub.DecryptScores(ctx, &vaultpb.DecryptScoresRequest{
		Token:            c.token,
		EncryptedBlobB64: encryptedBlobB64,
		TopK:             int32(topK),
	})
	if err != nil {
		return nil, MapGRPCError(err)
	}
	if msg := resp.GetError(); msg != "" {
		return nil, &Error{Code: ErrVaultInternal.Code, Message: "DecryptScores: " + msg, Retryable: true}
	}

	out := make([]ScoreEntry, 0, len(resp.GetResults()))
	for _, e := range resp.GetResults() {
		out = append(out, ScoreEntry{
			ShardIdx: e.GetShardIdx(),
			RowIdx:   e.GetRowIdx(),
			Score:    e.GetScore(),
		})
	}
	return out, nil
}

func (c *client) DecryptMetadata(ctx context.Context, encryptedMetadataList []string) ([]string, error) {
	ctx, cancel := withTimeout(c.authCtx(ctx), DefaultTimeout)
	defer cancel()

	resp, err := c.stub.DecryptMetadata(ctx, &vaultpb.DecryptMetadataRequest{
		Token:                 c.token,
		EncryptedMetadataList: encryptedMetadataList,
	})
	if err != nil {
		return nil, MapGRPCError(err)
	}
	if msg := resp.GetError(); msg != "" {
		return nil, &Error{Code: ErrVaultInternal.Code, Message: "DecryptMetadata: " + msg, Retryable: true}
	}
	return resp.GetDecryptedMetadata(), nil
}

func (c *client) HealthCheck(ctx context.Context) (bool, error) {
	ctx, cancel := withTimeout(ctx, HealthCheckTimeout)
	defer cancel()

	stub := grpc_health_v1.NewHealthClient(c.conn)
	resp, err := stub.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: ""})
	if err != nil {
		return false, MapGRPCError(err)
	}

	return resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING, nil
}

func (c *client) Endpoint() string { return c.endpoint }

func (c *client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
