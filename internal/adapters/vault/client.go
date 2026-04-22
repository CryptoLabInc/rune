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
	"time"
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

// client is the gRPC implementation.
type client struct {
	endpoint string
	token    string
	// TODO: grpc.ClientConn + RuneVaultServiceClient stub (needs external dep)
}

// NewClient — TODO: grpc.NewClient with MaxMessageLength opts + TLS creds
// (see spec/components/vault.md §TLS + §Keepalive).
func NewClient(endpoint, token string) (Client, error) {
	// TODO: implement
	return &client{endpoint: endpoint, token: token}, nil
}

// ValidateAgentDEK — Go-specific safety check (Python missing — see vault.md §agent_dek).
// Returns error if DEK length != 32. Non-retryable.
func ValidateAgentDEK(dek []byte) error {
	// TODO: return fmt.Errorf("vault: invalid agent_dek size %d (expected 32)", len(dek))
	return nil
}

// Stub implementations — all TODO.

func (c *client) GetPublicKey(ctx context.Context) (*Bundle, error) { return nil, nil }
func (c *client) DecryptScores(ctx context.Context, blob string, topK int) ([]ScoreEntry, error) {
	return nil, nil
}
func (c *client) DecryptMetadata(ctx context.Context, list []string) ([]string, error) {
	return nil, nil
}
func (c *client) HealthCheck(ctx context.Context) (bool, error) { return false, nil }
func (c *client) Endpoint() string                              { return c.endpoint }
func (c *client) Close() error                                  { return nil }
