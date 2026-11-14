package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
)

func (h *Handler) RestoreFile(ctx context.Context, snapshotID, filePath string, destinationPath, privateKey *string) error {
	_, err := h.repository.RestoreFile().Handle(ctx, app.RestoreFileCommand{
		SnapshotID:      snapshotID,
		FilePath:        filePath,
		RepositoryPath:  h.repository.Path(),
		DestinationPath: destinationPath,
		PrivateKeyPath:  privateKey,
	})
	return err
}
