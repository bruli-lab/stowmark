package snapshot

import (
	"context"
	"time"

	"github.com/bruli-lab/stowmark.git/internal/domain/repository"
)

type Create struct {
	sourceRepo   SourceRepository
	manifestRepo ManifestRepository
	objectRepo   ObjectRepository
	getConfigSvc *repository.GetConfig
}

func (c Create) Do(ctx context.Context, repoPath, sourcePath string) (*Result, error) {
	_, err := c.getConfigSvc.Get(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	source, err := c.sourceRepo.Explore(ctx, sourcePath)
	if err != nil {
		return nil, err
	}
	var size int64
	for i := range source.Files() {
		file := source.Files()[i]
		size += file.Size()
		hash, err := c.sourceRepo.CalculateHash(ctx, file.Path())
		if err != nil {
			return nil, err
		}
		file.AddHash(hash)
		if err := c.saveObject(ctx, file); err != nil {
			return nil, err
		}
		source.Files()[i] = file
	}
	man := NewManifest(source.Files(), time.Now().UTC(), source.AbsolutePath())
	if err := c.manifestRepo.Save(ctx, man); err != nil {
		return nil, err
	}
	return NewResult(man.Id(), len(man.Files()), size), nil
}

func (c Create) saveObject(ctx context.Context, file File) error {
	exist, err := c.objectRepo.AlreadyExists(ctx, &file)
	if err != nil {
		return err
	}
	if !exist {
		if err := c.objectRepo.Save(ctx, &file); err != nil {
			return err
		}
	}
	return nil
}

func NewCreate(
	sourceRepo SourceRepository,
	manifestRepo ManifestRepository,
	objRepo ObjectRepository,
	getConfigSvc *repository.GetConfig,
) *Create {
	return &Create{
		sourceRepo:   sourceRepo,
		manifestRepo: manifestRepo,
		objectRepo:   objRepo,
		getConfigSvc: getConfigSvc,
	}
}
