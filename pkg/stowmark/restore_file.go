package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
)

func (h *Handler) RestoreFile(ctx context.Context, snapshotID, filePath string, destinationPath, privateKey *string) error {
	obsv, err := builtObservability(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	decryptKeySvc := encryption.NewDecryptSymmetricKey(encrypt.NewSymmetricRepository(), disk.NewAsymmetricKeyPairRepository())
	svc := snapshot.NewRestoreFile(
		h.manifestRepository,
		h.objectRepository,
		h.folderRepository,
		decryptKeySvc,
	)
	tracerMdw := app.NewTracerCommandMiddleware(obsv.TracerProvider)
	handler := tracerMdw(app.NewRestoreFile(svc))
	_, err = handler.Handle(ctx, app.RestoreFileCommand{
		SnapshotID:      snapshotID,
		FilePath:        filePath,
		RepositoryPath:  h.repositoryPath,
		DestinationPath: destinationPath,
		PrivateKeyPath:  privateKey,
	})
	return err
}
