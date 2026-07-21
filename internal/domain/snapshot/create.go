package snapshot

import (
	"context"
	"time"
)

type Create struct {
	sourceRepo   SourceRepository
	manifestRepo ManifestRepository
}

func (c Create) Do(ctx context.Context, sourcePath, repositoryPath string) (*Result, error) {
	source, err := c.sourceRepo.Explore(ctx, sourcePath)
	if err != nil {
		return nil, err
	}
	for i := range source.Files() {
		file := source.Files()[i]
		hash, err := c.sourceRepo.CalculateHash(ctx, file.Path())
		if err != nil {
			return nil, err
		}
		file.AddHash(hash)
		source.Files()[i] = file
	}
	var size int64
	for _, file := range source.Files() {
		size += file.Size()
	}
	man := NewManifest(source.Files(), time.Now().UTC(), source.AbsolutePath())

	if err := c.manifestRepo.Save(ctx, repositoryPath, man); err != nil {
		return nil, err
	}

	return NewResult(man.Id(), len(man.Files()), size), nil
}

func NewCreate(sourceRepo SourceRepository, manifestRepo ManifestRepository) *Create {
	return &Create{sourceRepo: sourceRepo, manifestRepo: manifestRepo}
}
