package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
)

func (h *Handler) RestoreFile(ctx context.Context, snapshotID, filePath string, destinationPath, privateKey *string) error {
	decryptKeySvc := encryption.NewDecryptSymmetricKey(encrypt.NewSymmetricRepository(), disk.NewAsymmetricKeyPairRepository())
	svc := snapshot.NewRestoreFile(
		h.manifestRepository,
		h.objectRepository,
		h.folderRepository,
		decryptKeySvc,
	)
	return svc.Restore(ctx, snapshotID, filePath, h.repositoryPath, destinationPath, privateKey)
}
