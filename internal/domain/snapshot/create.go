package snapshot

import (
	"context"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

type Create struct {
	sourceRepo      SourceRepository
	manifestRepo    ManifestRepository
	objectRepo      ObjectRepository
	getConfigSvc    *repository.GetConfig
	hashCalculators *HashCalculatorFactory
}

func (c Create) Do(ctx context.Context, repoPath, sourcePath string) (*CreateResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	conf, err := c.getConfigSvc.Get(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	calculator, err := c.hashCalculators.Handle(conf.FormatVersion())
	if err != nil {
		return nil, err
	}

	source, err := c.sourceRepo.Explore(ctx, sourcePath)
	if err != nil {
		return nil, err
	}

	files := source.Files()

	var totalSize int64

	for i := range files {
		totalSize += files[i].Size()

		calculated, err := calculator.Calculate(ctx, &files[i], conf.Compression())
		if err != nil {
			return nil, err
		}

		if err := c.saveFileObjects(ctx, calculated, conf.Compression()); err != nil {
			return nil, err
		}

		files[i] = *calculated
	}

	manifest := NewManifest(newID(), files, time.Now().UTC(), source.AbsolutePath(), conf.Compression())

	if err := c.manifestRepo.Save(ctx, manifest); err != nil {
		return nil, err
	}

	return NewCreateResult(manifest.Id(), len(manifest.Files()), totalSize), nil
}

func (c Create) saveFileObjects(
	ctx context.Context,
	file *File,
	comp *repository.Compression,
) error {
	if len(file.Chunks()) == 0 {
		return c.saveObject(ctx, file.Path(), file.Hash(), comp)
	}

	for _, chunk := range file.Chunks() {
		if err := c.saveChunk(ctx, file.Path(), chunk, comp); err != nil {
			return err
		}
	}

	return nil
}

func (c Create) saveObject(
	ctx context.Context,
	filePath string,
	hash string,
	comp *repository.Compression,
) error {
	exists, err := c.objectRepo.AlreadyExists(ctx, hash)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	return c.objectRepo.Save(ctx, filePath, hash, comp)
}

func (c Create) saveChunk(ctx context.Context, filePath string, chunk Chunk, comp *repository.Compression) error {
	exists, err := c.objectRepo.AlreadyExists(ctx, chunk.Hash())
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	return c.objectRepo.SaveChunk(ctx, filePath, chunk.Hash(), chunk.Offset(), chunk.Size(), comp)
}

func NewCreate(sourceRepo SourceRepository, manifestRepo ManifestRepository, objectRepo ObjectRepository, getConfigSvc *repository.GetConfig) *Create {
	return &Create{
		sourceRepo:      sourceRepo,
		manifestRepo:    manifestRepo,
		objectRepo:      objectRepo,
		getConfigSvc:    getConfigSvc,
		hashCalculators: NewHashCalculatorFactory(sourceRepo),
	}
}
