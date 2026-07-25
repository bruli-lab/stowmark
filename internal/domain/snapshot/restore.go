package snapshot

import (
	"context"

	"github.com/bruli-lab/stowmark.git/internal/domain/repository"
)

type Restore struct {
	manifestRepo ManifestRepository
	getConfig    *repository.GetConfig
	objectRepo   ObjectRepository
}

func (r *Restore) Restore(ctx context.Context, repoPath, snapshotID string) (*Result, error) {
	man, err := r.manifestRepo.Get(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	conf, err := r.getConfig.Get(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	result := NewResult(snapshotID)
	for _, f := range man.Files() {
		if err := r.objectRepo.RestoreObject(ctx, conf.Compression(), &f); err != nil {
			result.AddFailed(*NewFailedResult(f.Path(), err.Error()))
			continue
		}
		result.AddSuccess()
	}
	return result, nil
}

func NewRestore(manifestRepo ManifestRepository, getConfig *repository.GetConfig, objectRepo ObjectRepository) *Restore {
	return &Restore{manifestRepo: manifestRepo, getConfig: getConfig, objectRepo: objectRepo}
}
