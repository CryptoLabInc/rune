package embedder

import (
	"context"
	"log/slog"
	"sync"
	"time"

	runedv1 "github.com/CryptoLabInc/runed/gen/runed/v1"
)

// infoCache caches the embedder Info RPC response.
//
// Spec: docs/v04/spec/components/embedder.md §Info 캐시.
//
// Behavior:
//   - Success: snapshot cached for the lifetime of the cache
//   - Error  : NOT cached. The next Get() re-attempts the RPC. Within the cooldown
//     window the most recent error is returned without an RPC

type infoCache struct {
	mu          sync.Mutex
	loaded      bool // sticky once true
	snap        InfoSnapshot
	lastErr     error
	lastAttempt time.Time // zero means "no attempt yet"
	svc         runedv1.RunedServiceClient
}

var infoRetryCooldown = 3 * time.Second

func (ic *infoCache) Get(ctx context.Context) (InfoSnapshot, error) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.loaded {
		return ic.snap, nil
	}
	if !ic.lastAttempt.IsZero() && time.Since(ic.lastAttempt) < infoRetryCooldown {
		return InfoSnapshot{}, ic.lastErr
	}
	ic.lastAttempt = time.Now()

	resp, err := ic.svc.Info(ctx, &runedv1.InfoRequest{})
	if err != nil {
		ic.lastErr = err
		return InfoSnapshot{}, err
	}
	ic.snap = InfoSnapshot{
		DaemonVersion: resp.GetDaemonVersion(),
		ModelIdentity: resp.GetModelIdentity(),
		VectorDim:     int(resp.GetVectorDim()),
		MaxTextLength: int(resp.GetMaxTextLength()),
		MaxBatchSize:  int(resp.GetMaxBatchSize()),
	}
	ic.loaded = true
	ic.lastErr = nil
	slog.Info("embedder info loaded",
		"daemon_version", ic.snap.DaemonVersion,
		"model_identity", ic.snap.ModelIdentity,
		"vector_dim", ic.snap.VectorDim,
		"max_batch_size", ic.snap.MaxBatchSize,
	)
	return ic.snap, nil
}

// Snapshot returns the cached value without triggering load.
// Returns zero InfoSnapshot if Get() has not yet succeeded.
func (ic *infoCache) Snapshot() InfoSnapshot {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	return ic.snap
}
