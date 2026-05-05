// Package vault is the Rune-Vault gRPC client.
// Spec: docs/v04/spec/components/vault.md.
// Python: mcp/adapter/vault_client.py (381 LoC).
//
// Responsibility:
//   - GetPublicKey: fetch FHE key bundle (+ envector creds, agent_dek)
//   - DecryptScores: Vault decrypts encrypted_blob → [{shard, row, score}]
//   - DecryptMetadata: Vault decrypts AES envelopes → plaintext JSON strings
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
)

// MaxMessageLength — 256MB for EvalKey (Python vault_client.py:L33).
// Applied on both MaxCallRecvMsgSize and MaxCallSendMsgSize.
const MaxMessageLength = 256 * 1024 * 1024

// DefaultTimeout — Python vault_client.py:L84 (all RPCs: 30s; health 5s override).
const DefaultTimeout = 30 * time.Second

// Bundle returned by GetPublicKey.
type Bundle struct {
	EncKey           []byte
	EvalKey          []byte
	EnvectorEndpoint string
	EnvectorAPIKey   string
	AgentID          string
	AgentDEK         []byte // MUST be exactly 32 bytes (AES-256) — Go adds this check (Python doesn't)
	KeyID            string
	IndexName        string
}

// ScoreEntry — DecryptScores output.
type ScoreEntry struct {
	ShardIdx int32
	RowIdx   int32
	Score    float64
}

// Client interface — implemented by gRPC client (and test mocks).
type Client interface {
	GetPublicKey(ctx context.Context) (*Bundle, error)
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
	pb       vaultpb.VaultServiceClient
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
	}

	if opts.TLSDisable {
		slog.Warn("vault: TLS disabled — gRPC traffic is unencrypted. Only use for local development.")
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else if opts.CACertPath != "" {
		creds, err := credentials.NewClientTLSFromFile(opts.CACertPath, "")
		if err != nil {
			return nil, fmt.Errorf("vault: failed to load CA cert %s: %w", opts.CACertPath, err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	} else {
		// System CA bundle
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
		pb:       vaultpb.NewVaultServiceClient(conn),
	}, nil
}

// ValidateAgentDEK — Go-specific safety check (Python missing — see vault.md §agent_dek).
// Returns error if DEK length != 32. Non-retryable.
func ValidateAgentDEK(dek []byte) error {
	if len(dek) != 32 {
		return fmt.Errorf("vault: invalid agent_dek size %d (expected 32)", len(dek))
	}
	return nil
}

// keyBundleJSON mirrors the map produced by Vault's buildBundle
// (rune-admin/vault/internal/server/grpc.go:131-153). EncKey and EvalKey
// are file-content strings (raw JSON of the FHE key files).
type keyBundleJSON struct {
	EncKey           string `json:"EncKey.json"`
	EvalKey          string `json:"EvalKey.json"`
	KeyID            string `json:"key_id"`
	IndexName        string `json:"index_name"`
	AgentID          string `json:"agent_id"`
	AgentDEK         string `json:"agent_dek"` // base64 of 32B
	EnvectorEndpoint string `json:"envector_endpoint"`
	EnvectorAPIKey   string `json:"envector_api_key"`
}

func (c *client) GetPublicKey(ctx context.Context) (*Bundle, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.pb.GetPublicKey(ctx, &vaultpb.GetPublicKeyRequest{Token: c.token})
	if err != nil {
		return nil, fmt.Errorf("vault: GetPublicKey rpc: %w", err)
	}
	if resp.GetError() != "" {
		return nil, fmt.Errorf("vault: GetPublicKey: %s", resp.GetError())
	}

	var b keyBundleJSON
	if err := json.Unmarshal([]byte(resp.GetKeyBundleJson()), &b); err != nil {
		return nil, fmt.Errorf("vault: parse key bundle json: %w", err)
	}

	dek, err := base64.StdEncoding.DecodeString(b.AgentDEK)
	if err != nil {
		return nil, fmt.Errorf("vault: decode agent_dek: %w", err)
	}
	if err := ValidateAgentDEK(dek); err != nil {
		return nil, err
	}

	return &Bundle{
		EncKey:           []byte(b.EncKey),
		EvalKey:          []byte(b.EvalKey),
		EnvectorEndpoint: b.EnvectorEndpoint,
		EnvectorAPIKey:   b.EnvectorAPIKey,
		AgentID:          b.AgentID,
		AgentDEK:         dek,
		KeyID:            b.KeyID,
		IndexName:        b.IndexName,
	}, nil
}

func (c *client) DecryptScores(ctx context.Context, blob string, topK int) ([]ScoreEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.pb.DecryptScores(ctx, &vaultpb.DecryptScoresRequest{
		Token:            c.token,
		EncryptedBlobB64: blob,
		TopK:             int32(topK),
	})
	if err != nil {
		return nil, fmt.Errorf("vault: DecryptScores rpc: %w", err)
	}
	if resp.GetError() != "" {
		return nil, fmt.Errorf("vault: DecryptScores: %s", resp.GetError())
	}

	out := make([]ScoreEntry, len(resp.GetResults()))
	for i, e := range resp.GetResults() {
		out[i] = ScoreEntry{
			ShardIdx: e.GetShardIdx(),
			RowIdx:   e.GetRowIdx(),
			Score:    e.GetScore(),
		}
	}
	return out, nil
}

func (c *client) DecryptMetadata(ctx context.Context, list []string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.pb.DecryptMetadata(ctx, &vaultpb.DecryptMetadataRequest{
		Token:                 c.token,
		EncryptedMetadataList: list,
	})
	if err != nil {
		return nil, fmt.Errorf("vault: DecryptMetadata rpc: %w", err)
	}
	if resp.GetError() != "" {
		return nil, fmt.Errorf("vault: DecryptMetadata: %s", resp.GetError())
	}
	return resp.GetDecryptedMetadata(), nil
}

func (c *client) HealthCheck(ctx context.Context) (bool, error) {
	// TODO: call grpc.health.v1.Health/Check
	return false, fmt.Errorf("vault: HealthCheck not yet implemented")
}

func (c *client) Endpoint() string { return c.endpoint }

func (c *client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
