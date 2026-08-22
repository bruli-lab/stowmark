package snapshot

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

var ErrMissingEncryptionConfigToEncrypt = errors.New("missing encryption config to encrypt")

var defaultCreateWorkers = runtime.GOMAXPROCS(0) * 2

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

	var totalSize int64

	workers := c.workers
	if workers <= 0 {
		workers = defaultCreateWorkers
	}
	if workers > len(files) {
		workers = len(files)
	}
	if workers < 1 {
		workers = 1
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)

	indexes := make(chan int)

	worker := func() {
		defer wg.Done()
		for i := range indexes {
			originalSize := files[i].Size()

			calculatedFile, err := calculator.Calculate(ctx, &files[i], conf.Compression())
			if err == nil {
				err = c.saveFileObjects(ctx, calculatedFile, conf.Compression(), symmetricKey, generation)
			}
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
					cancel()
				}
				mu.Unlock()
				continue
			}

			atomic.AddInt64(&totalSize, originalSize)
			files[i] = *calculatedFile
		}
	}

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go worker()
	}

feed:
	for i := range files {
		select {
		case indexes <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(indexes)
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	manifest := NewManifest(newID(), files, time.Now().UTC(), source.AbsolutePath(), conf.Compression())

	if err := c.manifestRepo.Save(ctx, manifest); err != nil {
		return nil, err
	}

	return NewCreateResult(manifest.Id(), len(manifest.Files()), totalSize), nil
}

func (c *Create) saveFileObjects(ctx context.Context, file *File, comp *repository.Compression, key []byte, generation uint64) error {
	if len(file.Chunks()) == 0 {
		return c.saveObject(ctx, file.Path(), file.Hash(), comp, key, generation)
	}

	for _, chunk := range file.Chunks() {
		if err := c.saveChunk(ctx, file.Path(), chunk, comp, key, generation); err != nil {
			return err
		}
	}

	return nil
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

func (c *Create) saveObject(ctx context.Context, filePath, hash string, comp *repository.Compression, key []byte, generation uint64) error {
	exists, err := c.objectRepo.AlreadyExists(ctx, hash, key, 0)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	return c.objectRepo.Save(ctx, filePath, hash, comp, key, generation)
}

func (c *Create) saveChunk(ctx context.Context, filePath string, chunk Chunk, comp *repository.Compression, symmetricKey []byte, generation uint64) error {
	exists, err := c.objectRepo.AlreadyExists(ctx, chunk.Hash(), symmetricKey, generation)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	return c.objectRepo.SaveChunk(ctx, filePath, chunk.Hash(), chunk.Offset(), chunk.Size(), comp, symmetricKey, generation)
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
