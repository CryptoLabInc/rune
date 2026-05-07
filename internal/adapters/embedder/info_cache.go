package embedder

import (
	"context"
	"log/slog"
	"sync"

	runedv1 "github.com/CryptoLabInc/runed/gen/runed/v1"
)

// infoCache caches the embedder Info RPC response.
//
// Spec: docs/v04/spec/components/embedder.md §Info 캐시.
//
// Behavior:
//   - sync.Once ensures first Get() triggers exactly one Info RPC
//   - subsequent Get() calls return cached snapshot (or cached error)
//   - no TTL — embedder config changes require daemon restart
//
// Logs a slog.Info breadcrumb on successful load (model_identity tracking
// for post-MVP re-embedding migration — D30). MVP scope is logging only;
// automatic model-change detection is deferred.
type infoCache struct {
	once sync.Once
	snap InfoSnapshot
	err  error
	svc  runedv1.RunedServiceClient
}

func (ic *infoCache) Get(ctx context.Context) (InfoSnapshot, error) {
	ic.once.Do(func() {
		resp, err := ic.svc.Info(ctx, &runedv1.InfoRequest{})
		if err != nil {
			ic.err = err
			return
		}
		ic.snap = InfoSnapshot{
			DaemonVersion: resp.GetDaemonVersion(),
			ModelIdentity: resp.GetModelIdentity(),
			VectorDim:     int(resp.GetVectorDim()),
			MaxTextLength: int(resp.GetMaxTextLength()),
			MaxBatchSize:  int(resp.GetMaxBatchSize()),
		}
		slog.Info("embedder info loaded",
			"daemon_version", ic.snap.DaemonVersion,
			"model_identity", ic.snap.ModelIdentity,
			"vector_dim", ic.snap.VectorDim,
			"max_batch_size", ic.snap.MaxBatchSize,
		)
	})
	return ic.snap, ic.err
}

// Snapshot returns the cached value without triggering load.
// Returns zero InfoSnapshot if Get() has never been called.
func (ic *infoCache) Snapshot() InfoSnapshot { return ic.snap }
