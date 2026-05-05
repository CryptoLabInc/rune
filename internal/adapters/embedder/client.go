// Package embedder is the gRPC client for the external embedder daemon.
// Spec: docs/v04/spec/components/embedder.md.
// Decision: D30 gRPC over Unix socket.
//
// rune-mcp does NOT spawn or manage the embedder. It connects as client.
// Socket path priority (spec/components/embedder.md §소켓 경로):
//  1. env RUNE_EMBEDDER_SOCKET
//  2. config.embedder.socket_path
//  3. embedder project convention default
//
// Retry policy (D7): [0, 500ms, 2s] × 3 on Unavailable / DeadlineExceeded /
// ResourceExhausted. Boot does NOT poll Health (D8) — first embed call drives.
package embedder

import (
	"context"
	"fmt"
	"time"

	runedv1 "github.com/CryptoLabInc/runed/gen/runed/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// RetryBackoffs — D7 (Python server.py timeout equivalent).
var RetryBackoffs = []time.Duration{
	0,
	500 * time.Millisecond,
	2 * time.Second,
}

// InfoSnapshot — cached via sync.Once on first call.
type InfoSnapshot struct {
	DaemonVersion string
	ModelIdentity string
	VectorDim     int
	MaxTextLength int
	MaxBatchSize  int
}

// HealthSnapshot — OK / LOADING / DEGRADED / SHUTTING_DOWN.
type HealthSnapshot struct {
	Status        string
	UptimeSeconds int64
	TotalRequests int64
}

// Client interface — thin wrapper over generated gRPC stub.
type Client interface {
	EmbedSingle(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Info(ctx context.Context) (InfoSnapshot, error)
	Health(ctx context.Context) (HealthSnapshot, error)
	Close() error
}

type client struct {
	sockPath string
	conn     *grpc.ClientConn
	pb       runedv1.RunedServiceClient
	info     *infoCache
}

// New dials the runed daemon over unix socket. The caller resolves sockPath
// (env RUNE_EMBEDDER_SOCKET > config.embedder.socket_path > default
// ~/.runed/embedding.sock per embedder.md §소켓 경로).
//
// grpc-go natively resolves "unix://" targets; no custom dialer is needed.
// TLS is unnecessary for UDS (kernel-mediated, same machine — embedder.md §Dial).
func New(sockPath string) (Client, error) {
	conn, err := grpc.NewClient(
		"unix://"+sockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("embedder: grpc dial %s: %w", sockPath, err)
	}
	pb := runedv1.NewRunedServiceClient(conn)
	return &client{
		sockPath: sockPath,
		conn:     conn,
		pb:       pb,
		info:     &infoCache{svc: pb},
	}, nil
}

// Stub implementations — replaced in subsequent commits.

func (c *client) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

func (c *client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, nil
}

func (c *client) Info(ctx context.Context) (InfoSnapshot, error)     { return InfoSnapshot{}, nil }
func (c *client) Health(ctx context.Context) (HealthSnapshot, error) { return HealthSnapshot{}, nil }
func (c *client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
