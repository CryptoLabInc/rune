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
// Key ownership:
//   - Vault owns EvalKey and SecKey; handles RegisterKeys/LoadKeys on the enVector server
//   - Plugin (this adapter) receives only EncKey from Vault
package envector

import (
	"context"
	"fmt"

	envector "github.com/CryptoLabInc/envector-go-sdk"
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
	OpenIndex(ctx context.Context) error               // opens (or creates) the server-side index
	GetIndexList(ctx context.Context) ([]string, error) // used by diagnostics + warmup
	Close() error
}

type ClientConfig struct {
	Endpoint  string            // enVector server address
	APIKey    string            // enVector access token (from Vault)
	KeyPath   string            // local EncKey directory (e.g. ~/.rune/keys/<key_id>/)
	KeyID     string            // key bundle identifier
	KeyDim    int               // FHE slot dimension (e.g. 1024)
	Preset    envector.Preset   // FHE param preset (zero = PresetIP0)
	EvalMode  envector.EvalMode // FHE eval strategy (zero = EvalModeRMP)
	IndexName string            // server-side index name
	Insecure  bool              // true for local dev (no TLS)
}

type client struct {
	sdk *envector.Client
	keys *envector.Keys
	idx  *envector.Index
	cfg  ClientConfig
}

// XXX: OpenIndex must be called separately before Insert/Score/GetMetadata are usable
func NewClient(cfg ClientConfig) (Client, error) {
	// SDK client (gRPC connection)
	clientOpts := []envector.ClientOption{
		envector.WithAddress(cfg.Endpoint),
		envector.WithAccessToken(cfg.APIKey),
	}
	if cfg.Insecure {
		clientOpts = append(clientOpts, envector.WithInsecure())
	}

	sdk, err := envector.NewClient(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("envector adapter: new client: %w", err)
	}

	// Open keys with KeyPartEnc
	// (Vault handles EvalKey and SecKey)
	keys, err := envector.OpenKeysFromFile(
		envector.WithKeyPath(cfg.KeyPath),
		envector.WithKeyID(cfg.KeyID),
		envector.WithKeyDim(cfg.KeyDim),
		envector.WithKeyPreset(cfg.Preset),
		envector.WithKeyEvalMode(cfg.EvalMode),
		envector.WithKeyParts(envector.KeyPartEnc),
	)
	if err != nil {
		_ = sdk.Close()
		return nil, fmt.Errorf("envector adapter: open keys: %w", err)
	}

	return &client{sdk: sdk, keys: keys, cfg: cfg}, nil
}

func (c *client) OpenIndex(ctx context.Context) error {
	// Open or create server-side index (key must be registered and loaded on enVector server)
	idx, err := c.sdk.Index(ctx,
		envector.WithIndexName(c.cfg.IndexName),
		envector.WithIndexKeys(c.keys),
	)
	if err != nil {
		return MapSDKError(err)
	}
	c.idx = idx
	return nil
}

func (c *client) Insert(ctx context.Context, req InsertRequest) (*InsertResult, error) {
	if c.idx == nil {
		return nil, &Error{Code: "ENVECTOR_NOT_ACTIVATED", Message: "OpenIndex must be called before Insert"}
	}

	// Adapter request to SDK request
	sdkReq := envector.InsertRequest{
		Vectors:  req.Vectors,
		Metadata: req.Metadata,
	}
	res, err := c.idx.Insert(ctx, sdkReq)
	if err != nil {
		return nil, MapSDKError(err)
	}

	return &InsertResult{ItemIDs: res.ItemIDs}, nil
}

func (c *client) Score(ctx context.Context, vec []float32) ([][]byte, error) {
	if c.idx == nil {
		return nil, &Error{Code: "ENVECTOR_NOT_ACTIVATED", Message: "OpenIndex must be called before Score"}
	}

	blobs, err := c.idx.Score(ctx, vec)
	if err != nil {
		return nil, MapSDKError(err)
	}

	return blobs, nil
}

func (c *client) GetMetadata(ctx context.Context, refs []MetadataRef, fields []string) ([]MetadataEntry, error) {
	if c.idx == nil {
		return nil, &Error{Code: "ENVECTOR_NOT_ACTIVATED", Message: "OpenIndex must be called before GetMetadata"}
	}

	// Adapter request to SDK request
	sdkRefs := make([]envector.MetadataRef, len(refs))
	for i, r := range refs {
		sdkRefs[i] = envector.MetadataRef{ShardIdx: r.ShardIdx, RowIdx: r.RowIdx}
	}

	mds, err := c.idx.GetMetadata(ctx, sdkRefs, fields)
	if err != nil {
		return nil, MapSDKError(err)
	}

	// SDK metadata{ID, Data} to adapter MetadataEntry{Data}
	out := make([]MetadataEntry, len(mds))
	for i, m := range mds {
		out[i] = MetadataEntry{Data: m.Data}
	}

	return out, nil
}

func (c *client) GetIndexList(ctx context.Context) ([]string, error) {
	list, err := c.sdk.GetIndexList(ctx)
	if err != nil {
		return nil, MapSDKError(err)
	}

	return list, nil
}

func (c *client) Close() error {
	var firstErr error

	// Release CGO key handles
	if c.keys != nil {
		if err := c.keys.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		c.keys = nil
	}

	// Release gRPC connection
	if c.sdk != nil {
		if err := c.sdk.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		c.sdk = nil
	}
	c.idx = nil

	return firstErr
}
