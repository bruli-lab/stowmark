package app

import (
	"context"
	"fmt"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const (
	PruneRepositoryCommandName = "prune-repository"
	defaultPruneWorkers        = 8
)

type PruneRepositoryCommand struct {
	RepositoryPath string
	DryRun         bool
}

func (p PruneRepositoryCommand) Name() string {
	return PruneRepositoryCommandName
}

type PruneRepository struct {
	folderRepo   FolderRepository
	manifestRepo ManifestRepository
	objectRepo   ObjectRepository
	locker       *RepositoryLocker
	workers      int
}

func (p *PruneRepository) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(PruneRepositoryCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(PruneRepositoryCommandName, cmd.Name())
	}

	var result *PruneRepositoryEvent
	err := p.locker.WithLock(ctx, lockKindFor(co.DryRun), func(ctx context.Context) error {
		pruned, err := p.prune(ctx, co)
		result = pruned
		return err
	})
	if err != nil {
		return nil, err
	}

	return []event.Event{result}, nil
}

func (p *PruneRepository) prune(ctx context.Context, co PruneRepositoryCommand) (*PruneRepositoryEvent, error) {
	conf, err := p.folderRepo.GetConfig(ctx, co.RepositoryPath)
	if err != nil {
		return nil, err
	}

	encrypted := conf.Encryption() != nil
	var generation uint64
	if encrypted {
		generation = conf.Encryption().Generation()
	}

	manifests, err := p.manifestRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	references := snapshot.NewReferences(manifests)

	stored, err := p.objectRepo.ListObjects(ctx, encrypted, generation)
	if err != nil {
		return nil, err
	}
	unreferenced := references.Unreferenced(stored)

	result := NewPruneRepositoryEvent(co.RepositoryPath, PruneStats{
		StoredObjects:       len(stored),
		ReferencedObjects:   references.Len(),
		UnreferencedObjects: len(unreferenced),
		MissingObjects:      len(references.Missing(stored)),
	}, co.DryRun)

	if co.DryRun {
		return result, nil
	}

	if err := p.deleteObjects(ctx, unreferenced, encrypted, generation); err != nil {
		return nil, err
	}

	return result, nil
}

func (p *PruneRepository) deleteObjects(ctx context.Context, hashes []string, encrypted bool, generation uint64) error {
	if len(hashes) == 0 {
		return nil
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(min(p.workers, len(hashes)))

	for _, hash := range hashes {
		if groupCtx.Err() != nil {
			break
		}

		group.Go(func() error {
			if err := p.objectRepo.DeleteObject(groupCtx, hash, encrypted, generation); err != nil {
				return fmt.Errorf("prune object %q: %w", hash, err)
			}
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}

	return ctx.Err()
}

func NewPruneRepository(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	locker *RepositoryLocker,
) *PruneRepository {
	return NewPruneRepositoryWithWorkers(folderRepo, manifestRepo, objectRepo, locker, defaultPruneWorkers)
}

func NewPruneRepositoryWithWorkers(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	locker *RepositoryLocker,
	workers int,
) *PruneRepository {
	if workers <= 0 {
		workers = defaultPruneWorkers
	}
	return &PruneRepository{
		folderRepo:   folderRepo,
		manifestRepo: manifestRepo,
		objectRepo:   objectRepo,
		locker:       locker,
		workers:      workers,
	}
}

type PruneStats struct {
	StoredObjects       int
	ReferencedObjects   int
	UnreferencedObjects int
	MissingObjects      int
}

type PruneRepositoryEvent struct {
	event.BasicEvent
	PruneStats
	DryRun bool
}

func NewPruneRepositoryEvent(repositoryPath string, stats PruneStats, dryRun bool) *PruneRepositoryEvent {
	return &PruneRepositoryEvent{
		BasicEvent: event.NewBasicEvent(PruneRepositoryCommandName, uuid.New(), repositoryPath),
		PruneStats: stats,
		DryRun:     dryRun,
	}
}
