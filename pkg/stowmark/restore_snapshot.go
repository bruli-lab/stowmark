package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

func (h *Handler) RestoreSnapshot(ctx context.Context, snapshotID string, destinationPath *string) (*Result, error) {
	svc := snapshot.NewRestore(
		h.manifestRepository,
		h.objectRepository,
	)
	result, err := svc.Restore(ctx, snapshotID, destinationPath)
	if err != nil {
		return nil, err
	}
	failed := make([]FailedResult, len(result.Failed()))
	for i, f := range result.Failed() {
		failed[i] = FailedResult{
			Path:   f.Path(),
			Reason: f.Reason(),
		}
	}
	return &Result{
		SnapshotID: result.SnapshotID(),
		TotalFiles: result.TotalFiles(),
		Failed:     failed,
		IsSuccess:  result.IsSuccess(),
	}, nil
}
