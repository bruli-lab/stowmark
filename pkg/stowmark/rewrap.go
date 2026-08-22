package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

func (h *Handler) Rewrap(ctx context.Context, oldPrivateKeyPat, newPublicKeyPath string) error {
	svc := repository.NewRewrap(h.folderRepository, h.symmetricKeyRepository, h.asymmetricKeyPaiRepository)
	return svc.Do(ctx, h.repositoryPath, oldPrivateKeyPat, newPublicKeyPath)
}
