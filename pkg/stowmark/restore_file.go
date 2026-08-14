package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

func (h *Handler) RestoreFile(ctx context.Context, snapshotID, filePath string) error {
	svc := snapshot.NewRestoreFile(
		h.manifestRepository,
		h.objectRepository,
	)
	return svc.Restore(ctx, snapshotID, filePath)
}
