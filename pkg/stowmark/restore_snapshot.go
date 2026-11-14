package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

func (h *Handler) RestoreSnapshot(ctx context.Context, snapshotID string, destinationPath, privateKey *string) (*Result, error) {
	events, err := h.repository.RestoreSnapshot().Handle(ctx, app.RestoreSnapshotCommand{
		SnapshotID:      snapshotID,
		RepositoryPath:  h.repository.Path(),
		PrivateKeyPath:  privateKey,
		DestinationPath: destinationPath,
	})
	if err != nil {
		return nil, err
	}
	result, err := app.SingleEvent[*snapshot.SnapshotRestored](events)
	if err != nil {
		return nil, err
	}
	return newResult(result.SnapshotID(), result.TotalFiles(), result.Failed(), result.IsSuccess()), nil
}
