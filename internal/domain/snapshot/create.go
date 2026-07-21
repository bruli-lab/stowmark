package snapshot

import (
	"context"
	"fmt"
	"time"
)

type Create struct {
	sourceRepo SourceRepository
}

func (c Create) Do(ctx context.Context, sourcePath string) (*Result, error) {
	source, err := c.sourceRepo.Explore(ctx, sourcePath)
	if err != nil {
		return nil, err
	}
	for _, file := range source.Files() {
		hash, err := c.sourceRepo.CalculateHash(ctx, file.Path())
		if err != nil {
			return nil, err
		}
		file.AddHash(hash)
		fmt.Printf("file: %s, size: %v \n hash: %s \n", file.Path(), file.Size(), file.Hash())
	}
	var size int64
	for _, file := range source.Files() {
		size += file.Size()
	}
	man := NewManifest(source.Files(), time.Now().UTC(), source.AbsolutePath())

	return NewResult(man.Id(), len(man.Files()), size), nil
}

func NewCreate(explorer SourceRepository) *Create {
	return &Create{sourceRepo: explorer}
}
