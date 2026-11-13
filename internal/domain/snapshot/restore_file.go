package snapshot

import (
	"context"
	"errors"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

var ErrFileNotInSnapshot = errors.New("file not found in snapshot")

type RestoreFile struct {
	manifestRepo  ManifestRepository
	objectRepo    ObjectRepository
	symKeyHandler *SymmetricKeyHandler
}

func (r *RestoreFile) Restore(
	ctx context.Context,
	snapshotID, filePath, repositoryPath string,
	destinationPath, privateKeyPath *string,
) error {
	data, err := r.symKeyHandler.Handle(ctx, privateKeyPath, repositoryPath)
	if err != nil {
		return err
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
		return err
	}
	file, err := r.findFile(man, filePath)
	if err != nil {
		return err
	}
	if destinationPath != nil {
		if err := file.ChangeSourcePath(man.Source(), *destinationPath); err != nil {
			return err
		}
	}
	if err := r.objectRepo.RestoreObject(ctx, man.compression, file, symmetricKey, generation); err != nil {
		return err
	}
	return nil
}

func (r *RestoreFile) findFile(man *Manifest, filePath string) (*File, error) {
	for _, f := range man.Files() {
		if f.Path() == filePath {
			return &f, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrFileNotInSnapshot, filePath)
}

func NewRestoreFile(
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	folderRepo repository.FolderRepository,
	decryptKeySvc *encryption.DecryptSymmetricKey,
) *RestoreFile {
	return &RestoreFile{
		manifestRepo:  manifestRepo,
		objectRepo:    objectRepo,
		symKeyHandler: newSymmetricKeyHandler(folderRepo, decryptKeySvc),
	}
}
