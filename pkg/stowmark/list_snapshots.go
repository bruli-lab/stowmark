package stowmark

import (
	"context"
	"time"

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
	items, err := snapshot.NewListing(h.manifestRepository).List(ctx)
	if err != nil {
		return nil, err
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
