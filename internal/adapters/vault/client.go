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
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Applied on both MaxCallRecvMsgSize and MaxCallSendMsgSize.
const MaxMessageLength = 16 * 1024 * 1024

// DefaultTimeout — Python vault_client.py:L84 (all RPCs: 30s; health 5s override).
const DefaultTimeout = 30 * time.Second

// Returned by GetAgentManifest (GetAgentManifestResponse.manifest_json)
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
	AgentDEK         string `json:"agent_dek"` // hex-encoded 32-byte key
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

func decodeAgentDEK(hexStr string) ([]byte, error) {
	if hexStr == "" {
		return nil, fmt.Errorf("vault: agent_dek is empty")
	}

	// Decodes
	b := make([]byte, len(hexStr)/2)
	for i := 0; i < len(b); i++ {
		hi := unhex(hexStr[2*i])
		lo := unhex(hexStr[2*i+1])
		if hi < 0 || lo < 0 {
			return nil, fmt.Errorf("vault: agent_dek contains non-hex character at position %d", 2*i)
		}
		b[i] = byte(hi<<4 | lo)
	}

	// Validate 32-byte length
	if len(b) != 32 {
		return nil, fmt.Errorf("vault: invalid agent_dek size %d (expected 32)", len(b))
	}

	return b, nil
}

func unhex(c byte) int {
	switch {
	case '0' <= c && c <= '9':
		return int(c - '0')
	case 'a' <= c && c <= 'f':
		return int(c - 'a' + 10)
	case 'A' <= c && c <= 'F':
		return int(c - 'A' + 10)
	}
	return -1
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
	// TODO: grpc.ClientConn + VaultServiceClient stub (needs external dep)
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
	return &client{endpoint: normalized, token: token, conn: conn}, nil
}

// Stub implementations — all TODO.

func (c *client) GetAgentManifest(ctx context.Context) (*Bundle, error) { return nil, nil }
func (c *client) DecryptScores(ctx context.Context, blob string, topK int) ([]ScoreEntry, error) {
	// TODO: call VaultService.DecryptScores RPC
	return nil, fmt.Errorf("vault: DecryptScores not yet implemented (needs proto codegen)")
}

func (c *client) DecryptMetadata(ctx context.Context, list []string) ([]string, error) {
	// TODO: call VaultService.DecryptMetadata RPC
	return nil, fmt.Errorf("vault: DecryptMetadata not yet implemented (needs proto codegen)")
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
