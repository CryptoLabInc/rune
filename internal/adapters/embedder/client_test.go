package embedder_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	runedv1 "github.com/CryptoLabInc/runed/gen/runed/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/envector/rune-go/internal/adapters/embedder"
)

// fakeRuned implements RunedServiceServer in-process. Each RPC delegates to
// a per-test func; tests configure those before calling the client.
type fakeRuned struct {
	runedv1.UnimplementedRunedServiceServer

	embedFn      func(*runedv1.EmbedRequest) (*runedv1.EmbedResponse, error)
	embedBatchFn func(*runedv1.EmbedBatchRequest) (*runedv1.EmbedBatchResponse, error)
	infoFn       func(*runedv1.InfoRequest) (*runedv1.InfoResponse, error)
	healthFn     func(*runedv1.HealthRequest) (*runedv1.HealthResponse, error)

	infoCalls   int32 // atomic — Info should be invoked exactly once across the lifetime of an infoCache
	embedCalls  int32 // atomic — used by retry test to count attempts
	embedBatchCalls int32 // atomic — used by batch-split test
}

func (f *fakeRuned) Embed(_ context.Context, req *runedv1.EmbedRequest) (*runedv1.EmbedResponse, error) {
	atomic.AddInt32(&f.embedCalls, 1)
	if f.embedFn != nil {
		return f.embedFn(req)
	}
	return nil, status.Error(codes.Unimplemented, "Embed not stubbed")
}

func (f *fakeRuned) EmbedBatch(_ context.Context, req *runedv1.EmbedBatchRequest) (*runedv1.EmbedBatchResponse, error) {
	atomic.AddInt32(&f.embedBatchCalls, 1)
	if f.embedBatchFn != nil {
		return f.embedBatchFn(req)
	}
	return nil, status.Error(codes.Unimplemented, "EmbedBatch not stubbed")
}

func (f *fakeRuned) Info(_ context.Context, req *runedv1.InfoRequest) (*runedv1.InfoResponse, error) {
	atomic.AddInt32(&f.infoCalls, 1)
	if f.infoFn != nil {
		return f.infoFn(req)
	}
	return &runedv1.InfoResponse{
		DaemonVersion: "0.1.0-test",
		ModelIdentity: "qwen3-test",
		VectorDim:     1024,
		MaxTextLength: 8192,
		MaxBatchSize:  4,
	}, nil
}

func (f *fakeRuned) Health(_ context.Context, req *runedv1.HealthRequest) (*runedv1.HealthResponse, error) {
	if f.healthFn != nil {
		return f.healthFn(req)
	}
	return &runedv1.HealthResponse{Status: runedv1.HealthResponse_STATUS_OK}, nil
}

func startFakeRuned(t *testing.T) (*fakeRuned, embedder.Client) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	fake := &fakeRuned{}
	runedv1.RegisterRunedServiceServer(srv, fake)
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

	return fake, embedder.NewBufconnClient(conn)
}

func TestEmbedSingle_HappyPath(t *testing.T) {
	fake, c := startFakeRuned(t)
	fake.embedFn = func(req *runedv1.EmbedRequest) (*runedv1.EmbedResponse, error) {
		if req.GetText() != "hello" {
			t.Errorf("text: got %q, want hello", req.GetText())
		}
		return &runedv1.EmbedResponse{Vector: []float32{0.1, 0.2, 0.3}}, nil
	}

	v, err := c.EmbedSingle(context.Background(), "hello")
	if err != nil {
		t.Fatalf("EmbedSingle: %v", err)
	}
	if len(v) != 3 || v[0] != 0.1 {
		t.Errorf("vector: got %v", v)
	}
}

func TestEmbedSingle_RetryThenSuccess(t *testing.T) {
	fake, c := startFakeRuned(t)
	var attempts int32
	fake.embedFn = func(*runedv1.EmbedRequest) (*runedv1.EmbedResponse, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 2 {
			return nil, status.Error(codes.Unavailable, "transient")
		}
		return &runedv1.EmbedResponse{Vector: []float32{1}}, nil
	}

	v, err := c.EmbedSingle(context.Background(), "hi")
	if err != nil {
		t.Fatalf("retry should succeed: %v", err)
	}
	if len(v) != 1 || v[0] != 1 {
		t.Errorf("vector: got %v", v)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("attempts: got %d, want 2", atomic.LoadInt32(&attempts))
	}
}

func TestEmbedSingle_NonRetryableErrorReturnsImmediately(t *testing.T) {
	fake, c := startFakeRuned(t)
	fake.embedFn = func(*runedv1.EmbedRequest) (*runedv1.EmbedResponse, error) {
		return nil, status.Error(codes.InvalidArgument, "text too long")
	}

	_, err := c.EmbedSingle(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code: got %v, want InvalidArgument", st.Code())
	}
	if atomic.LoadInt32(&fake.embedCalls) != 1 {
		t.Errorf("attempts: got %d, want 1 (no retry)", atomic.LoadInt32(&fake.embedCalls))
	}
}

func TestEmbedBatch_NoSplitWhenLEMax(t *testing.T) {
	fake, c := startFakeRuned(t)
	// Info default: MaxBatchSize=4. Pass 3 texts → single chunk.
	fake.embedBatchFn = func(req *runedv1.EmbedBatchRequest) (*runedv1.EmbedBatchResponse, error) {
		if len(req.GetTexts()) != 3 {
			t.Errorf("texts in single chunk: got %d, want 3", len(req.GetTexts()))
		}
		return &runedv1.EmbedBatchResponse{Embeddings: []*runedv1.EmbedResponse{
			{Vector: []float32{1}}, {Vector: []float32{2}}, {Vector: []float32{3}},
		}}, nil
	}

	out, err := c.EmbedBatch(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(out) != 3 {
		t.Errorf("output: got %d, want 3", len(out))
	}
	if atomic.LoadInt32(&fake.embedBatchCalls) != 1 {
		t.Errorf("EmbedBatch RPC calls: got %d, want 1", atomic.LoadInt32(&fake.embedBatchCalls))
	}
}

func TestEmbedBatch_SplitsWhenAboveMax(t *testing.T) {
	fake, c := startFakeRuned(t)
	fake.embedBatchFn = func(req *runedv1.EmbedBatchRequest) (*runedv1.EmbedBatchResponse, error) {
		// Echo a vector per input
		emb := make([]*runedv1.EmbedResponse, len(req.GetTexts()))
		for i, txt := range req.GetTexts() {
			emb[i] = &runedv1.EmbedResponse{Vector: []float32{float32(len(txt))}}
		}
		return &runedv1.EmbedBatchResponse{Embeddings: emb}, nil
	}

	// MaxBatchSize=4 (default Info), 9 texts → 3 chunks (4+4+1)
	texts := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}
	out, err := c.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(out) != 9 {
		t.Errorf("output: got %d, want 9", len(out))
	}
	if atomic.LoadInt32(&fake.embedBatchCalls) != 3 {
		t.Errorf("EmbedBatch RPC calls: got %d, want 3 (4+4+1)", atomic.LoadInt32(&fake.embedBatchCalls))
	}
}

func TestEmbedBatch_RespCountMismatchErrors(t *testing.T) {
	fake, c := startFakeRuned(t)
	fake.embedBatchFn = func(req *runedv1.EmbedBatchRequest) (*runedv1.EmbedBatchResponse, error) {
		// Return ONE less embedding than requested — contract violation
		return &runedv1.EmbedBatchResponse{Embeddings: []*runedv1.EmbedResponse{
			{Vector: []float32{1}},
		}}, nil
	}

	_, err := c.EmbedBatch(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error on resp count mismatch")
	}
	if !strings.Contains(err.Error(), "expected 2 embeddings") {
		t.Errorf("error: got %q", err.Error())
	}
}

func TestEmbedBatch_EmptyInput(t *testing.T) {
	_, c := startFakeRuned(t)
	out, err := c.EmbedBatch(context.Background(), nil)
	if err != nil {
		t.Errorf("empty input: got error %v, want nil", err)
	}
	if out != nil {
		t.Errorf("empty input: got %v, want nil", out)
	}
}

func TestInfo_CachesAcrossCalls(t *testing.T) {
	fake, c := startFakeRuned(t)
	for i := 0; i < 5; i++ {
		_, err := c.Info(context.Background())
		if err != nil {
			t.Fatalf("Info: %v", err)
		}
	}
	if got := atomic.LoadInt32(&fake.infoCalls); got != 1 {
		t.Errorf("Info RPC calls: got %d, want 1 (sync.Once cache)", got)
	}
}

func TestInfo_CachesError(t *testing.T) {
	fake, c := startFakeRuned(t)
	fake.infoFn = func(*runedv1.InfoRequest) (*runedv1.InfoResponse, error) {
		return nil, status.Error(codes.Unavailable, "down")
	}
	_, err1 := c.Info(context.Background())
	if err1 == nil {
		t.Fatal("first Info: expected error")
	}
	_, err2 := c.Info(context.Background())
	if err2 == nil {
		t.Fatal("second Info: expected cached error")
	}
	// Both calls return the same cached error; only one RPC fires.
	if got := atomic.LoadInt32(&fake.infoCalls); got != 1 {
		t.Errorf("Info RPC calls: got %d, want 1 (error caches too)", got)
	}
}

func TestHealth_StatusEnumMapping(t *testing.T) {
	cases := []struct {
		grpcStatus runedv1.HealthResponse_Status
		want       string
	}{
		{runedv1.HealthResponse_STATUS_OK, "OK"},
		{runedv1.HealthResponse_STATUS_LOADING, "LOADING"},
		{runedv1.HealthResponse_STATUS_DEGRADED, "DEGRADED"},
		{runedv1.HealthResponse_STATUS_SHUTTING_DOWN, "SHUTTING_DOWN"},
		{runedv1.HealthResponse_STATUS_UNSPECIFIED, "UNSPECIFIED"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			fake, c := startFakeRuned(t)
			fake.healthFn = func(*runedv1.HealthRequest) (*runedv1.HealthResponse, error) {
				return &runedv1.HealthResponse{
					Status:        tc.grpcStatus,
					UptimeSeconds: 42,
					TotalRequests: 7,
				}, nil
			}
			snap, err := c.Health(context.Background())
			if err != nil {
				t.Fatalf("Health: %v", err)
			}
			if snap.Status != tc.want {
				t.Errorf("Status: got %q, want %q", snap.Status, tc.want)
			}
			if snap.UptimeSeconds != 42 || snap.TotalRequests != 7 {
				t.Errorf("counters: got %+v", snap)
			}
		})
	}
}

func TestEmbedSingle_RetriesExhaust(t *testing.T) {
	fake, c := startFakeRuned(t)
	fake.embedFn = func(*runedv1.EmbedRequest) (*runedv1.EmbedResponse, error) {
		return nil, status.Error(codes.Unavailable, "permanently down")
	}

	// Use a context with deadline so the third backoff doesn't drag the test.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.EmbedSingle(ctx, "x")
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if !strings.Contains(err.Error(), "all retries exhausted") {
		t.Errorf("error: got %q, want 'all retries exhausted'", err.Error())
	}
	// 3 attempts: backoff [0, 500ms, 2s] × 3 → 3 calls
	if got := atomic.LoadInt32(&fake.embedCalls); got != 3 {
		t.Errorf("Embed RPC calls: got %d, want 3", got)
	}
}

func TestEmbedSingle_RespectsCancelledContext(t *testing.T) {
	fake, c := startFakeRuned(t)
	fake.embedFn = func(*runedv1.EmbedRequest) (*runedv1.EmbedResponse, error) {
		return nil, status.Error(codes.Unavailable, "transient")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before call

	_, err := c.EmbedSingle(ctx, "x")
	if err == nil {
		t.Fatal("expected error from cancelled ctx")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("error: got %v, want context.Canceled", err)
	}
}
