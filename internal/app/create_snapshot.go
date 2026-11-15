//nolint:dupl // CQS command handlers intentionally share the same adapter structure.
package app

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"golang.org/x/sync/errgroup"
)

const CreateSnapshotCommandName = "create-snapshot"

var defaultCreateWorkers = runtime.GOMAXPROCS(0) * 2

type CreateSnapshotCommand struct {
	RepositoryPath  string
	SourcePath      string
	PrivateKey      *string
	Exclude         []string
	SkipIgnoreFiles bool
	Progress        Progress
}

func (c *CreateSnapshotCommand) Name() string {
	return CreateSnapshotCommandName
}

type CreateSnapshot struct {
	sourceRepo   SourceRepository
	manifestRepo ManifestRepository
	objectRepo   ObjectRepository
	folderRepo   FolderRepository
	keys         keyring
	budget       *memoryBudget
	locker       *RepositoryLocker
	workers      int
}

func (c *CreateSnapshot) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(*CreateSnapshotCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(CreateSnapshotCommandName, cmd.Name())
	}
	return withLockedEvent(ctx, c.locker, repository.SharedLock, func(ctx context.Context) (*snapshot.SnapshotCreated, error) {
		return c.create(ctx, co)
	})
}

func (c *CreateSnapshot) create(ctx context.Context, co *CreateSnapshotCommand) (*snapshot.SnapshotCreated, error) {
	conf, err := c.folderRepo.GetConfig(ctx, co.RepositoryPath)
	if err != nil {
		return nil, err
	}

	calculate, err := newHashCalculator(c.sourceRepo, conf.FormatVersion())
	if err != nil {
		return nil, err
	}

	rules, err := c.excludeRules(ctx, co)
	if err != nil {
		return nil, err
	}

	explored, err := c.sourceRepo.Explore(ctx, co.SourcePath, rules)
	if err != nil {
		return nil, err
	}
	source := explored.ForFormat(conf.FormatVersion())

	codec, err := c.keys.forConfig(ctx, conf, co.PrivateKey)
	if err != nil {
		return nil, err
	}

	files := source.Files()
	items, bytes := contentTotals(files)
	progress := startProgress(co.Progress, ProgressBackingUp, items, bytes)
	defer progress.Finish()

	var (
		totalSize  atomic.Int64
		savedFiles atomic.Int64
	)

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(c.workers)

	for i := range files {
		if groupCtx.Err() != nil {
			break
		}

		group.Go(func() error {
			hashed, err := calculate(groupCtx, files[i], codec)
			if err != nil {
				return err
			}
			files[i] = hashed

			savedSize, err := c.saveFileObjects(groupCtx, &files[i], codec, progress)
			if err != nil {
				return err
			}

			totalSize.Add(savedSize)
			if savedSize > 0 {
				savedFiles.Add(1)
			}
			progress.Advance(1, 0)

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	manifest := snapshot.NewManifest(snapshot.NewID(), files, time.Now().UTC(), source.AbsolutePath(), conf.Compression())

	if err := c.manifestRepo.Save(ctx, manifest, codec); err != nil {
		return nil, err
	}
	return snapshot.NewSnapshotCreated(manifest, int(savedFiles.Load()), totalSize.Load(), source.Skipped()), nil
}

func (c *CreateSnapshot) excludeRules(ctx context.Context, co *CreateSnapshotCommand) (*snapshot.ExcludeRules, error) {
	var filePatterns []string
	if !co.SkipIgnoreFiles {
		patterns, err := c.sourceRepo.IgnorePatterns(ctx, co.SourcePath)
		if err != nil {
			return nil, err
		}
		filePatterns = patterns
	}
	rules, err := snapshot.NewExcludeRules(filePatterns)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", snapshot.IgnoreFileName, err)
	}
	rules, err = rules.WithOverrides(co.Exclude)
	if err != nil {
		return nil, fmt.Errorf("parse exclude patterns: %w", err)
	}
	if co.SkipIgnoreFiles {
		return rules.WithoutIgnoreFiles(), nil
	}
	return rules, nil
}

func (c *CreateSnapshot) saveFileObjects(ctx context.Context, file *snapshot.File, codec repository.Codec, progress Progress) (int64, error) {
	if !file.HasContent() {
		return 0, nil
	}
	if len(file.Chunks()) == 0 {
		size, err := c.saveObject(ctx, file, codec)
		if err == nil {
			progress.Advance(0, file.Size())
		}
		return size, err
	}

	var savedSize int64
	for _, chunk := range file.Chunks() {
		size, err := c.saveChunk(ctx, file.Path(), chunk, codec)
		if err != nil {
			return 0, err
		}
		savedSize += size
		progress.Advance(0, chunk.Size())
	}

	return savedSize, nil
}

func (c *CreateSnapshot) saveObject(ctx context.Context, file *snapshot.File, codec repository.Codec) (int64, error) {
	exists, err := c.objectRepo.AlreadyExists(ctx, file.Hash(), codec)
	if err != nil || exists {
		return 0, err
	}

	release, err := c.budget.reserve(ctx, codec, file.Size())
	if err != nil {
		return 0, err
	}
	defer release()
	if err := c.objectRepo.Save(ctx, file.Path(), file.Hash(), codec); err != nil {
		return 0, err
	}
	return file.Size(), nil
}

func (c *CreateSnapshot) saveChunk(ctx context.Context, filePath string, chunk snapshot.Chunk, codec repository.Codec) (int64, error) {
	exists, err := c.objectRepo.AlreadyExists(ctx, chunk.Hash(), codec)
	if err != nil || exists {
		return 0, err
	}

	release, err := c.budget.reserve(ctx, codec, chunk.Size())
	if err != nil {
		return 0, err
	}
	defer release()
	if err := c.objectRepo.SaveChunk(ctx, filePath, chunk, codec); err != nil {
		return 0, err
	}
	return chunk.Size(), nil
}

func NewCreateSnapshot(
	sourceRepo SourceRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	folderRepo FolderRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *CreateSnapshot {
	return NewCreateSnapshotWithWorkers(sourceRepo, manifestRepo, objectRepo, folderRepo, asymmetricRepo, symmetricRepo, locker, defaultCreateWorkers)
}

func NewCreateSnapshotWithWorkers(
	sourceRepo SourceRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	folderRepo FolderRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
	workers int,
) *CreateSnapshot {
	if workers <= 0 {
		workers = defaultCreateWorkers
	}
	return &CreateSnapshot{
		sourceRepo:   sourceRepo,
		manifestRepo: manifestRepo,
		objectRepo:   objectRepo,
		folderRepo:   folderRepo,
		keys:         newKeyring(folderRepo, asymmetricRepo, symmetricRepo),
		budget:       newMemoryBudget(defaultMemoryBudget),
		locker:       locker,
		workers:      workers,
	}
}
