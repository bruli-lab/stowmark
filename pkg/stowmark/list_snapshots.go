package stowmark

import (
	"context"
	"fmt"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

type SnapshotSummary struct {
	ID        string
	CreatedAt time.Time
	FileCount int
	TotalSize int64
	Source    string
}

func (h *Handler) ListSnapshots(ctx context.Context) ([]SnapshotSummary, error) {
	obsv, err := builtObservability(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	svc := snapshot.NewListing(h.manifestRepository)
	tracerMdw := app.NewTracerQueryMiddleware(obsv.TracerProvider)
	handler := tracerMdw(app.NewSnapshotList(svc))
	resultQuery, err := handler.Handle(ctx, app.SnaphotListQuery{})
	if err != nil {
		return nil, err
	}
	items, ok := resultQuery.([]snapshot.ManifestResume)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", resultQuery)
	}

	result := make([]SnapshotSummary, len(items))
	for i, item := range items {
		result[i] = SnapshotSummary{
			ID:        item.Id(),
			CreatedAt: item.CreatedAt(),
			FileCount: item.Files(),
			TotalSize: item.Size(),
			Source:    item.Source(),
		}
	}

	return result, nil
}
