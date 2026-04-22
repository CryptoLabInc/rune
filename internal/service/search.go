package service

import (
	"context"
	"fmt"

	"github.com/envector/rune-go/internal/adapters/embedder"
	"github.com/envector/rune-go/internal/adapters/envector"
	"github.com/envector/rune-go/internal/adapters/vault"
	"github.com/envector/rune-go/internal/domain"
)

// SearchByID — shared helper used by delete_capture (lifecycle §5) and, if
// needed, by recall. Python: agents/retriever/searcher.py:L561-567.
//
// Hack: embed "ID: {record_id}" as query and search top-5 via standard pipeline,
// then filter results by exact record_id match. Relies on envector similarity
// surfacing the target record for its self-embedding. Kept as-is under D25/D27
// bit-identical principle.
//
// Signature takes adapters as params to avoid owning a struct — both
// LifecycleService and RecallService can invoke the same helper.
//
// TODO:
//
//	vec, err := embedder.EmbedSingle(ctx, fmt.Sprintf("ID: %s", recordID))
//	hits, err := searchSingleStandalone(ctx, vec, 5, vaultClient, envClient, indexName)
//	for _, h := range hits {
//	    if h.RecordID == recordID { return &h, nil }
//	}
//	return nil, nil  // not found — caller returns InvalidInputError
func SearchByID(
	ctx context.Context,
	embedderClient embedder.Client,
	vaultClient vault.Client,
	envClient envector.Client,
	indexName string,
	recordID string,
) (*domain.SearchHit, error) {
	query := fmt.Sprintf("ID: %s", recordID)
	_ = query
	// TODO: per lifecycle.md §5 searchByID
	return nil, nil
}
