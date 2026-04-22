// Package envector wraps the envector-go SDK and implements AES-256-CTR envelope
// (Seal/Open). Spec: docs/v04/spec/components/envector.md.
// Python: mcp/adapter/envector_sdk.py (387 LoC).
//
// Responsibility boundary (critical — per 3-layer rule in envector.md §Recall):
//   - envector SDK: opaque string for metadata (never interprets)
//   - this adapter: Seal/Open (AES envelope) + SDK method wrapping
//   - service layer: orchestrates Seal on capture, calls Vault.DecryptMetadata on recall
//     The adapter does NOT call Vault in the decrypt path.
//
// Pending: envector-go SDK OpenKeysFromFile conditional PR (Q4). MVP blocked on
// libevi binary availability; mock backend used until then.
package envector

import (
	"context"
)

// MetadataRef — {shard_idx, row_idx} ref for GetMetadata/Remind.
type MetadataRef struct {
	ShardIdx uint64
	RowIdx   uint64
}

// MetadataEntry — server response; Data is opaque string (AES envelope or plain JSON or legacy base64).
type MetadataEntry struct {
	Data string
}

// InsertRequest — batch capture (N vectors + N envelopes).
type InsertRequest struct {
	Vectors  [][]float32
	Metadata []string // AES envelope strings; SDK stores verbatim
}

// InsertResult — server-assigned IDs.
type InsertResult struct {
	ItemIDs []int64
}

// Client interface — SDK wrapper.
type Client interface {
	Insert(ctx context.Context, req InsertRequest) (*InsertResult, error)
	Score(ctx context.Context, vec []float32) ([][]byte, error)
	GetMetadata(ctx context.Context, refs []MetadataRef, fields []string) ([]MetadataEntry, error)
	ActivateKeys(ctx context.Context) error
	GetIndexList(ctx context.Context) ([]string, error) // used by diagnostics + warmup
	Close() error
}

type client struct {
	endpoint string
	apiKey   string
	// TODO: envector-go SDK Client + Keys + Index handles
}

// NewClient — TODO: call envector.NewClient + OpenKeysFromFile + ActivateKeys + Index.
func NewClient(endpoint, apiKey string) (Client, error) {
	// TODO
	return &client{endpoint: endpoint, apiKey: apiKey}, nil
}

// Stub implementations.

func (c *client) Insert(ctx context.Context, req InsertRequest) (*InsertResult, error) {
	return nil, nil
}
func (c *client) Score(ctx context.Context, vec []float32) ([][]byte, error) { return nil, nil }
func (c *client) GetMetadata(ctx context.Context, refs []MetadataRef, fields []string) ([]MetadataEntry, error) {
	return nil, nil
}
func (c *client) ActivateKeys(ctx context.Context) error             { return nil }
func (c *client) GetIndexList(ctx context.Context) ([]string, error) { return nil, nil }
func (c *client) Close() error                                       { return nil }
