//go:build integration

package envector

import (
	"context"
	"os"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Integration tests require a running enVector server with keys already
// registered and loaded by Vault
//
// Run with:
//   ENVECTOR_TEST_ENDPOINT=host:port ENVECTOR_TEST_TOKEN=... \
//   ENVECTOR_TEST_KEY_PATH=~/.rune/keys/test-key \
//   ENVECTOR_TEST_KEY_ID=test-key \
//   go test -tags integration ./internal/adapters/envector/...
//
// These tests the real SDK with real FHE crypto + gRPC transport
// ---------------------------------------------------------------------------

func testConfig(t *testing.T) ClientConfig {
	t.Helper()

	endpoint := os.Getenv("ENVECTOR_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("ENVECTOR_TEST_ENDPOINT not set; skipping integration test")
	}
	token := os.Getenv("ENVECTOR_TEST_TOKEN")
	keyPath := os.Getenv("ENVECTOR_TEST_KEY_PATH")
	if keyPath == "" {
		t.Skip("ENVECTOR_TEST_KEY_PATH not set; skipping integration test")
	}
	keyID := os.Getenv("ENVECTOR_TEST_KEY_ID")
	if keyID == "" {
		keyID = "test-key"
	}
	indexName := os.Getenv("ENVECTOR_TEST_INDEX")
	if indexName == "" {
		indexName = "rune-test"
	}
	dimStr := os.Getenv("ENVECTOR_TEST_DIM")
	dim := 128
	if dimStr != "" {
		dim = 0
		for _, c := range dimStr {
			dim = dim*10 + int(c-'0')
		}
		if dim <= 0 {
			dim = 128
		}
	}

	return ClientConfig{
		Endpoint:  endpoint,
		APIKey:    token,
		KeyPath:   keyPath,
		KeyID:     keyID,
		KeyDim:    dim,
		IndexName: indexName,
		Insecure:  os.Getenv("ENVECTOR_TEST_INSECURE") == "1",
	}
}

func TestIntegration_NewClient(t *testing.T) {
	cfg := testConfig(t)
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()
}

func TestIntegration_OpenIndex(t *testing.T) {
	cfg := testConfig(t)
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Assumes Vault has already registered and loaded keys on the server
	if err := c.OpenIndex(ctx); err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
}

func TestIntegration_GetIndexList(t *testing.T) {
	cfg := testConfig(t)
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.OpenIndex(ctx); err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}

	list, err := c.GetIndexList(ctx)
	if err != nil {
		t.Fatalf("GetIndexList: %v", err)
	}
	t.Logf("Indices: %v", list)

	found := false
	for _, name := range list {
		if name == cfg.IndexName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("index %q not found in list %v", cfg.IndexName, list)
	}
}

func TestIntegration_InsertScoreGetMetadata(t *testing.T) {
	cfg := testConfig(t)
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := c.OpenIndex(ctx); err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}

	// Test vector with metadata
	dim := cfg.KeyDim
	vec := make([]float32, dim)
	for i := range vec {
		vec[i] = float32(i) / float32(dim)
	}
	meta := `{"test":"integration"}`
	res, err := c.Insert(ctx, InsertRequest{
		Vectors:  [][]float32{vec},
		Metadata: []string{meta},
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if len(res.ItemIDs) != 1 {
		t.Fatalf("Insert returned %d ItemIDs, want 1", len(res.ItemIDs))
	}
	t.Logf("Inserted item ID: %d", res.ItemIDs[0])

	// Score with the same vector
	blobs, err := c.Score(ctx, vec)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if len(blobs) == 0 {
		t.Fatal("Score returned 0 blobs")
	}
	t.Logf("Score returned %d blobs, first blob size: %d bytes", len(blobs), len(blobs[0]))

	// Cannot decrypt scores here
	for i, b := range blobs {
		if len(b) == 0 {
			t.Errorf("blob[%d] is empty", i)
		}
	}
}
