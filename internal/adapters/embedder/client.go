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
	SocketPath() string
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
	return newWithConn(sockPath, conn), nil
}

// NewBufconnClient wraps an existing *grpc.ClientConn (e.g., from
// google.golang.org/grpc/test/bufconn) so tests can exercise the same RPC
// path without needing a real unix socket / runed daemon.
func NewBufconnClient(conn *grpc.ClientConn) Client {
	return newWithConn("bufconn", conn)
}

func newWithConn(sockPath string, conn *grpc.ClientConn) *client {
	pb := runedv1.NewRunedServiceClient(conn)
	return &client{
		sockPath: sockPath,
		conn:     conn,
		pb:       pb,
		info:     &infoCache{svc: pb},
	}
}

func (c *client) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	resp, err := retry(ctx, func(ctx context.Context) (*runedv1.EmbedResponse, error) {
		return c.pb.Embed(ctx, &runedv1.EmbedRequest{Text: text})
	})
	if err != nil {
		return nil, err
	}
	return resp.GetVector(), nil
}

// EmbedBatch splits len(texts) > Info.MaxBatchSize into chunks and submits
// each chunk via embedBatchOnce. Order is preserved.
func (c *client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	info, err := c.info.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("embedder: load Info before EmbedBatch: %w", err)
	}

	if info.MaxBatchSize <= 0 || len(texts) <= info.MaxBatchSize {
		return c.embedBatchOnce(ctx, texts)
	}

	out := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += info.MaxBatchSize {
		end := i + info.MaxBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		chunk, err := c.embedBatchOnce(ctx, texts[i:end])
		if err != nil {
			return nil, err
		}
		out = append(out, chunk...)
	}
	return out, nil
}

func (c *client) embedBatchOnce(ctx context.Context, texts []string) ([][]float32, error) {
	resp, err := retry(ctx, func(ctx context.Context) (*runedv1.EmbedBatchResponse, error) {
		return c.pb.EmbedBatch(ctx, &runedv1.EmbedBatchRequest{Texts: texts})
	})
	if err != nil {
		return nil, err
	}
	if len(resp.GetEmbeddings()) != len(texts) {
		return nil, fmt.Errorf("embedder: expected %d embeddings, got %d", len(texts), len(resp.GetEmbeddings()))
	}
	out := make([][]float32, len(resp.GetEmbeddings()))
	for i, e := range resp.GetEmbeddings() {
		out[i] = e.GetVector()
	}
	return out, nil
}

func (c *client) Info(ctx context.Context) (InfoSnapshot, error) { return c.info.Get(ctx) }

func (c *client) SocketPath() string { return c.sockPath }

// Health issues a Health RPC. Status maps proto enum (STATUS_OK / STATUS_LOADING /
// STATUS_DEGRADED / STATUS_SHUTTING_DOWN / STATUS_UNSPECIFIED) to the
// "STATUS_"-stripped string the spec documents (OK / LOADING / DEGRADED /
// SHUTTING_DOWN / UNSPECIFIED).
//
// Health is NOT retried — D8 says first embed call drives connectivity; Health
// is a diagnostic tool surface (vault_status, diagnostics).
func (c *client) Health(ctx context.Context) (HealthSnapshot, error) {
	resp, err := c.pb.Health(ctx, &runedv1.HealthRequest{})
	if err != nil {
		return HealthSnapshot{}, MapGRPCError(err)
	}
	return HealthSnapshot{
		Status:        statusName(resp.GetStatus()),
		UptimeSeconds: resp.GetUptimeSeconds(),
		TotalRequests: resp.GetTotalRequests(),
	}, nil
}

func statusName(s runedv1.HealthResponse_Status) string {
	switch s {
	case runedv1.HealthResponse_STATUS_OK:
		return "OK"
	case runedv1.HealthResponse_STATUS_LOADING:
		return "LOADING"
	case runedv1.HealthResponse_STATUS_DEGRADED:
		return "DEGRADED"
	case runedv1.HealthResponse_STATUS_SHUTTING_DOWN:
		return "SHUTTING_DOWN"
	default:
		return "UNSPECIFIED"
	}
}
func (c *client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
