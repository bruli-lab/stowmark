package snapshot

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"golang.org/x/sync/errgroup"
)

var ErrMissingEncryptionConfigToEncrypt = errors.New("missing encryption config to encrypt")

var defaultCreateWorkers = runtime.GOMAXPROCS(0) * 2

type FileSavedSize int64

func (s FileSavedSize) Int64() int64 {
	return int64(s)
}

type Create struct {
	sourceRepo      SourceRepository
	manifestRepo    ManifestRepository
	objectRepo      ObjectRepository
	getConfigSvc    *repository.GetConfig
	hashCalculators *HashCalculatorFactory
	decryptSvc      *encryption.DecryptSymmetricKey
	workers         int
}

func (c *Create) Do(ctx context.Context, repoPath, sourcePath string, privateKey *string) (*CreateResult, error) {
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

	symmetricKey, err := c.getSymmetricKey(ctx, conf.Encryption(), privateKey)
	if err != nil {
		return nil, err
	}
	var generation uint64
	if len(symmetricKey) != 0 {
		generation = conf.Encryption().Generation()
	}

	workers := c.workers
	if workers <= 0 {
		workers = defaultCreateWorkers
	}

	var (
		totalSize  atomic.Int64
		savedFiles atomic.Int64
	)

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(workers)

	for i := range files {
		if groupCtx.Err() != nil {
			break
		}

		group.Go(func() error {
			calculatedFile, err := calculator.Calculate(groupCtx, &files[i], conf.Compression())
			if err != nil {
				return err
			}

			savedSize, err := c.saveFileObjects(groupCtx, calculatedFile, conf.Compression(), symmetricKey, generation)
			if err != nil {
				return err
			}

			totalSize.Add(savedSize.Int64())
			if savedSize.Int64() > 0 {
				savedFiles.Add(1)
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	manifest := NewManifest(newID(), files, time.Now().UTC(), source.AbsolutePath(), conf.Compression())

	if err := c.manifestRepo.Save(ctx, manifest); err != nil {
		return nil, err
	}
	return NewCreateResult(manifest.Id(), int(savedFiles.Load()), totalSize.Load()), nil
}

func (c *Create) saveFileObjects(ctx context.Context, file *File, comp *repository.Compression, key []byte, generation uint64) (FileSavedSize, error) {
	if len(file.Chunks()) == 0 {
		return c.saveObject(ctx, file, comp, key, generation)
	}

	var savedChunksSize FileSavedSize
	for _, chunk := range file.Chunks() {
		savedSize, err := c.saveChunk(ctx, file.Path(), chunk, comp, key, generation)
		if err != nil {
			return 0, err
		}
		savedChunksSize += savedSize
	}

	return savedChunksSize, nil
}

func (c *Create) getSymmetricKey(ctx context.Context, encryptionConfig *encryption.EncryptionConfig, privateKeyPath *string) ([]byte, error) {
	switch {
	case encryptionConfig == nil && privateKeyPath != nil:
		return nil, ErrMissingEncryptionConfigToEncrypt
	case encryptionConfig != nil && privateKeyPath != nil:
		decryptKey, err := c.decryptSvc.Decrypt(ctx, *privateKeyPath, encryptionConfig.EncryptedKey())
		if err != nil {
			return nil, err
		}
		return decryptKey, nil
	default:
	}
	return nil, nil
}

func (c *Create) saveObject(ctx context.Context, file *File, comp *repository.Compression, key []byte, generation uint64) (FileSavedSize, error) {
	exists, err := c.objectRepo.AlreadyExists(ctx, file.Hash(), key, generation)
	if err != nil {
		return 0, err
	}

	if exists {
		return 0, nil
	}

	if err := c.objectRepo.Save(ctx, file.Path(), file.Hash(), comp, key, generation); err != nil {
		return 0, err
	}
	return FileSavedSize(file.Size()), nil
}

func (c *Create) saveChunk(ctx context.Context, filePath string, chunk Chunk, comp *repository.Compression, symmetricKey []byte, generation uint64) (FileSavedSize, error) {
	exists, err := c.objectRepo.AlreadyExists(ctx, chunk.Hash(), symmetricKey, generation)
	if err != nil {
		return 0, err
	}

	if exists {
		return 0, nil
	}

	if err := c.objectRepo.SaveChunk(ctx, filePath, chunk.Hash(), chunk.Offset(), chunk.Size(), comp, symmetricKey, generation); err != nil {
		return 0, err
	}
	return FileSavedSize(chunk.Size()), nil
}

func NewCreate(
	sourceRepo SourceRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	getConfigSvc *repository.GetConfig,
	decryptSvc *encryption.DecryptSymmetricKey,
) *Create {
	return NewCreateWithWorkers(sourceRepo, manifestRepo, objectRepo, getConfigSvc, decryptSvc, defaultCreateWorkers)
}

func NewCreateWithWorkers(
	sourceRepo SourceRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	getConfigSvc *repository.GetConfig,
	decryptSvc *encryption.DecryptSymmetricKey,
	workers int,
) *Create {
	return &Create{
		sourceRepo:      sourceRepo,
		manifestRepo:    manifestRepo,
		objectRepo:      objectRepo,
		getConfigSvc:    getConfigSvc,
		hashCalculators: NewHashCalculatorFactory(sourceRepo),
		decryptSvc:      decryptSvc,
		workers:         workers,
	}
}
