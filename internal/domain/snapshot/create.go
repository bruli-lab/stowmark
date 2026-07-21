package snapshot

import (
	"context"
	"time"
)

type Create struct {
	sourceRepo   SourceRepository
	manifestRepo ManifestRepository
	objectRepo   ObjectRepository
}

func (c Create) Do(ctx context.Context, sourcePath string) (*Result, error) {
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
		if err := c.objectRepo.Save(ctx, &file); err != nil {
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

func NewCreate(
	sourceRepo SourceRepository,
	manifestRepo ManifestRepository,
	objRepo ObjectRepository,
) *Create {
	return &Create{
		sourceRepo:   sourceRepo,
		manifestRepo: manifestRepo,
		objectRepo:   objRepo,
	}
}
