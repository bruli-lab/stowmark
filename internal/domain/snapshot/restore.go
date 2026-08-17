package snapshot

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

type Restore struct {
	manifestRepo  ManifestRepository
	objectRepo    ObjectRepository
	symKeyHandler *SymmetricKeyHandler
}

func (r *Restore) Restore(
	ctx context.Context,
	snapshotID, repositoryPath string,
	destinationPath, privateKeyPath *string,
) (*Result, error) {
	data, err := r.symKeyHandler.Handle(ctx, privateKeyPath, repositoryPath)
	if err != nil {
		return nil, err
	}
	var (
		symmetricKey []byte
		generation   uint64
	)
	if data != nil {
		symmetricKey = data.SymmetricKey
		generation = data.Generation
	}

	man, err := r.manifestRepo.Get(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	result := NewResult(snapshotID)
	for _, f := range man.Files() {
		if destinationPath != nil {
			f.ChangeSourcePath(man.Source(), *destinationPath)
		}
		if err := r.objectRepo.RestoreObject(ctx, man.compression, &f, symmetricKey, generation); err != nil {
			result.AddFailed(*NewFailedResult(f.Path(), err.Error()))
			continue
		}
		result.AddSuccess()
	}
	return result, nil
}

func NewRestore(
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	folderRepo repository.FolderRepository,
	decryptKeySvc *encryption.DecryptSymmetricKey,
) *Restore {
	return &Restore{
		manifestRepo:  manifestRepo,
		objectRepo:    objectRepo,
		symKeyHandler: newSymmetricKeyHandler(folderRepo, decryptKeySvc),
	}
}
