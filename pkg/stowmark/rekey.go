package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

func (h *Handler) ReKey(ctx context.Context, privateKeyPath, publicKeyPath string) error {
	svc := snapshot.NewReKey(h.folderRepository, h.symmetricKeyRepository, h.asymmetricKeyPaiRepository, h.objectRepository)
	err := svc.Do(ctx, h.repositoryPath, privateKeyPath, publicKeyPath)
	if err != nil {
		return err
	}
	return nil
}
