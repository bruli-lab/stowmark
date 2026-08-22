package snapshot

import (
	"context"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

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
		file.ChangeSourcePath(man.Source(), *destinationPath)
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
	return nil, NewRestoreFileError("file not found: " + filePath + "")
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

type RestoreFileError struct {
	msg string
}

func (r RestoreFileError) Error() string {
	return fmt.Sprintf("failed to restore file: %s", r.msg)
}

func NewRestoreFileError(msg string) *RestoreFileError {
	return &RestoreFileError{msg: msg}
}
