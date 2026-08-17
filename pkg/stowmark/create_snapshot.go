package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

type CreateResult struct {
	ID        string
	FileCount int
	TotalSize int64
}

func (h *Handler) CreateSnapshot(ctx context.Context, source string, privateKey *string) (*CreateResult, error) {
	svc := snapshot.NewCreate(
		h.sourceRepository,
		h.manifestRepository,
		h.objectRepository,
		repository.NewGetConfig(h.folderRepository),
		encryption.NewDecryptSymmetricKey(h.symmetricKeyRepository, h.asymmetricKeyPaiRepository),
	)
	result, err := svc.Do(ctx, h.repositoryPath, source, privateKey)
	if err != nil {
		return nil, err
	}
	return &CreateResult{
		ID:        result.Id(),
		FileCount: result.FileCount(),
		TotalSize: result.TotalSize(),
	}, nil
}
