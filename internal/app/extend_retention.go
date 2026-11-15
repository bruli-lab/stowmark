package app

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

const ExtendRetentionCommandName = "extend-retention"

type ExtendRetentionCommand struct {
	RepositoryPath string
	PrivateKeyPath *string
	Progress       Progress
}

func (e ExtendRetentionCommand) Name() string {
	return ExtendRetentionCommandName
}

type ExtendRetention struct {
	folderRepo    FolderRepository
	manifestRepo  ManifestRepository
	retentionRepo RetentionRepository
	keys          keyring
	walker        retentionWalker
	locker        *RepositoryLocker
}

func (e *ExtendRetention) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(ExtendRetentionCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(ExtendRetentionCommandName, cmd.Name())
	}
	extended, err := withLockedResult(ctx, e.locker, repository.SharedLock, func(ctx context.Context) (*repository.RetentionExtended, error) {
		return e.extend(ctx, co)
	})
	if err != nil {
		return nil, err
	}
	if extended.Extended() == 0 {
		return []event.Event{}, nil
	}
	return []event.Event{extended}, nil
}

func (e *ExtendRetention) extend(ctx context.Context, co ExtendRetentionCommand) (*repository.RetentionExtended, error) {
	conf, err := e.folderRepo.GetConfig(ctx, co.RepositoryPath)
	if err != nil {
		return nil, err
	}
	lock := conf.ObjectLock()
	if lock == nil {
		return nil, repository.ErrObjectLockDisabled
	}
	codec, err := e.keys.forConfig(ctx, conf, co.PrivateKeyPath)
	if err != nil {
		return nil, err
	}
	manifests, err := e.manifestRepo.List(ctx, codec)
	if err != nil {
		return nil, err
	}

	items := snapshot.RetainedItems(manifests, conf)
	now := time.Now()
	retention := lock.RetentionFrom(now)
	var checked, extended atomic.Int64

	progress := startProgress(co.Progress, ProgressRetaining, len(items), 0)
	defer progress.Finish()

	err = e.walker.walk(ctx, items, progress, func(ctx context.Context, item repository.RetainedItem, until time.Time) error {
		checked.Add(1)
		if !lock.NeedsExtension(until, now) {
			return nil
		}
		if err := e.retentionRepo.Retain(ctx, item, retention); err != nil {
			return fmt.Errorf("extend the retention of %s %q: %w", item.Kind(), item.ID(), err)
		}
		extended.Add(1)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return conf.RetentionExtended(int(checked.Load()), int(extended.Load()), retention.Until()), nil
}

func NewExtendRetention(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	retentionRepo RetentionRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *ExtendRetention {
	return &ExtendRetention{
		folderRepo:    folderRepo,
		manifestRepo:  manifestRepo,
		retentionRepo: retentionRepo,
		keys:          newKeyring(folderRepo, asymmetricRepo, symmetricRepo),
		walker:        newRetentionWalker(retentionRepo),
		locker:        locker,
	}
}
