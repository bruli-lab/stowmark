package app

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const CreateSnapshotCommandName = "create-snapshot"

var defaultCreateWorkers = runtime.GOMAXPROCS(0) * 2

type CreateSnapshotCommand struct {
	RepositoryPath string
	SourcePath     string
	PrivateKey     *string
}

func (c CreateSnapshotCommand) Name() string {
	return CreateSnapshotCommandName
}

type CreateSnapshot struct {
	sourceRepo   SourceRepository
	manifestRepo ManifestRepository
	objectRepo   ObjectRepository
	folderRepo   FolderRepository
	keys         keyring
	locker       *RepositoryLocker
	workers      int
}

func (c *CreateSnapshot) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(CreateSnapshotCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(CreateSnapshotCommandName, cmd.Name())
	}
	var result *snapshot.CreateResult
	err := c.locker.WithLock(ctx, repository.SharedLock, func(ctx context.Context) error {
		created, err := c.create(ctx, co)
		result = created
		return err
	})
	if err != nil {
		return nil, err
	}
	return []event.Event{
		NewCreateSnapshotEvent(result.ID(), result.FileCount(), result.TotalSize()),
	}, nil
}

func (c *CreateSnapshot) create(ctx context.Context, co CreateSnapshotCommand) (*snapshot.CreateResult, error) {
	conf, err := c.folderRepo.GetConfig(ctx, co.RepositoryPath)
	if err != nil {
		return nil, err
	}

	calculate, err := newHashCalculator(c.sourceRepo, conf.FormatVersion())
	if err != nil {
		return nil, err
	}

	source, err := c.sourceRepo.Explore(ctx, co.SourcePath)
	if err != nil {
		return nil, err
	}

	key, err := c.keys.forConfig(ctx, conf.Encryption(), co.PrivateKey)
	if err != nil {
		return nil, err
	}

	files := source.Files()

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
			if err := calculate(groupCtx, &files[i], conf.Compression()); err != nil {
				return err
			}

			savedSize, err := c.saveFileObjects(groupCtx, &files[i], conf.Compression(), key)
			if err != nil {
				return err
			}

			totalSize.Add(savedSize)
			if savedSize > 0 {
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

	manifest := snapshot.NewManifest(snapshot.NewID(), files, time.Now().UTC(), source.AbsolutePath(), conf.Compression())

	if err := c.manifestRepo.Save(ctx, manifest); err != nil {
		return nil, err
	}
	return snapshot.NewCreateResult(manifest.ID(), int(savedFiles.Load()), totalSize.Load()), nil
}

func (c *CreateSnapshot) saveFileObjects(ctx context.Context, file *snapshot.File, comp *repository.Compression, key objectKey) (int64, error) {
	if len(file.Chunks()) == 0 {
		return c.saveObject(ctx, file, comp, key)
	}

	var savedSize int64
	for _, chunk := range file.Chunks() {
		size, err := c.saveChunk(ctx, file.Path(), chunk, comp, key)
		if err != nil {
			return 0, err
		}
		savedSize += size
	}

	return savedSize, nil
}

func (c *CreateSnapshot) saveObject(ctx context.Context, file *snapshot.File, comp *repository.Compression, key objectKey) (int64, error) {
	exists, err := c.objectRepo.AlreadyExists(ctx, file.Hash(), key.symmetricKey, key.generation)
	if err != nil || exists {
		return 0, err
	}

	if err := c.objectRepo.Save(ctx, file.Path(), file.Hash(), comp, key.symmetricKey, key.generation); err != nil {
		return 0, err
	}
	return file.Size(), nil
}

func (c *CreateSnapshot) saveChunk(ctx context.Context, filePath string, chunk snapshot.Chunk, comp *repository.Compression, key objectKey) (int64, error) {
	exists, err := c.objectRepo.AlreadyExists(ctx, chunk.Hash(), key.symmetricKey, key.generation)
	if err != nil || exists {
		return 0, err
	}

	if err := c.objectRepo.SaveChunk(ctx, filePath, chunk.Hash(), chunk.Offset(), chunk.Size(), comp, key.symmetricKey, key.generation); err != nil {
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
		locker:       locker,
		workers:      workers,
	}
}

type CreateSnapshotEvent struct {
	event.BasicEvent
	SnapshotID string
	FileCount  int
	TotalSize  int64
}

func NewCreateSnapshotEvent(
	snapshotID string,
	fileCount int,
	totalSize int64,
) *CreateSnapshotEvent {
	return &CreateSnapshotEvent{
		BasicEvent: event.NewBasicEvent(CreateSnapshotCommandName, uuid.New(), snapshotID),
		SnapshotID: snapshotID,
		FileCount:  fileCount,
		TotalSize:  totalSize,
	}
}
