package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

func (h *Handler) RestoreFile(ctx context.Context, snapshotID, filePath string, destinationPath, privateKey *string) error {
	obsv, err := buildObservability(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	evtr := app.NewEventsTracing()
	mdw, err := middlewares.BuildCommandMiddlewares(obsv, evtr)
	if err != nil {
		return err
	}
	handler := mdw(app.NewRestoreFile(h.manifestRepository, h.objectRepository, h.folderRepository, h.asymmetricKeyPairRepository, h.symmetricKeyRepository, h.locker))
	_, err = handler.Handle(ctx, app.RestoreFileCommand{
		SnapshotID:      snapshotID,
		FilePath:        filePath,
		RepositoryPath:  h.repositoryPath,
		DestinationPath: destinationPath,
		PrivateKeyPath:  privateKey,
	})
	return err
}
