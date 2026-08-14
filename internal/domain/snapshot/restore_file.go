package snapshot

import (
	"context"
	"fmt"
)

type RestoreFile struct {
	manifestRepo ManifestRepository
	objectRepo   ObjectRepository
}

func (r *RestoreFile) Restore(ctx context.Context, snapshotID, filePath string, destinationPath *string) error {
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
	if err := r.objectRepo.RestoreObject(ctx, man.compression, file); err != nil {
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

func NewRestoreFile(manifestRepo ManifestRepository, objectRepo ObjectRepository) *RestoreFile {
	return &RestoreFile{manifestRepo: manifestRepo, objectRepo: objectRepo}
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
