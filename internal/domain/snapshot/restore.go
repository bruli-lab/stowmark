package snapshot

import (
	"context"
)

type Restore struct {
	manifestRepo ManifestRepository
	objectRepo   ObjectRepository
}

func (r *Restore) Restore(ctx context.Context, snapshotID string, destinationPath *string) (*Result, error) {
	man, err := r.manifestRepo.Get(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	result := NewResult(snapshotID)
	for _, f := range man.Files() {
		if destinationPath != nil {
			f.ChangePath(*destinationPath)
		}
		if err := r.objectRepo.RestoreObject(ctx, man.compression, &f); err != nil {
			result.AddFailed(*NewFailedResult(f.Path(), err.Error()))
			continue
		}
		result.AddSuccess()
	}
	return result, nil
}

func NewRestore(manifestRepo ManifestRepository, objectRepo ObjectRepository) *Restore {
	return &Restore{manifestRepo: manifestRepo, objectRepo: objectRepo}
}
