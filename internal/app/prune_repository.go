package app

import (
	"context"
	"fmt"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"golang.org/x/sync/errgroup"
)

const (
	PruneRepositoryCommandName = "prune-repository"
	defaultPruneWorkers        = 8
)

type PruneRepositoryCommand struct {
	RepositoryPath string
}

func (p PruneRepositoryCommand) Name() string {
	return PruneRepositoryCommandName
}

type PruneRepository struct {
	planner       prunePlanner
	objectRepo    ObjectRepository
	encryptedRepo EncryptedObjectRepository
	locker        *RepositoryLocker
	workers       int
}

func (p *PruneRepository) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(PruneRepositoryCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(PruneRepositoryCommandName, cmd.Name())
	}

	events := make([]event.Event, 0)
	err := p.locker.WithLock(ctx, repository.ExclusiveLock, func(ctx context.Context) error {
		target, err := p.planner.plan(ctx, co.RepositoryPath)
		if err != nil {
			return err
		}
		repositoryID := target.config.ID().String()
		if len(target.plan.Missing()) > 0 {
			events = append(events, target.plan.MissingDetected(repositoryID))
		}
		if err := p.deleteObjects(ctx, target); err != nil {
			return err
		}
		if len(target.plan.Unreferenced()) > 0 {
			events = append(events, target.plan.Pruned(repositoryID))
		}
		if err := p.deleteGenerations(ctx, target); err != nil {
			return err
		}
		if len(target.plan.OrphanGenerations()) > 0 {
			events = append(events, target.plan.GenerationsRemoved(repositoryID))
		}
		temporaries, err := p.objectRepo.DeleteTemporaryObjects(ctx)
		if err != nil {
			return fmt.Errorf("delete temporary objects: %w", err)
		}
		if temporaries > 0 {
			events = append(events, snapshot.NewTemporariesRemoved(repositoryID, temporaries))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return events, nil
}

func (p *PruneRepository) deleteObjects(ctx context.Context, target *pruneTarget) error {
	hashes := target.plan.Unreferenced()
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
			if err := p.objectRepo.DeleteObject(groupCtx, hash, target.encrypted, target.generation); err != nil {
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

func (p *PruneRepository) deleteGenerations(ctx context.Context, target *pruneTarget) error {
	for _, generation := range target.plan.OrphanGenerations() {
		if err := p.encryptedRepo.DeleteEncryptedGeneration(ctx, generation); err != nil {
			return fmt.Errorf("prune key generation %d: %w", generation, err)
		}
	}
	return nil
}

func NewPruneRepository(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	encryptedRepo EncryptedObjectRepository,
	locker *RepositoryLocker,
) *PruneRepository {
	return NewPruneRepositoryWithWorkers(folderRepo, manifestRepo, objectRepo, encryptedRepo, locker, defaultPruneWorkers)
}

func NewPruneRepositoryWithWorkers(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	encryptedRepo EncryptedObjectRepository,
	locker *RepositoryLocker,
	workers int,
) *PruneRepository {
	if workers <= 0 {
		workers = defaultPruneWorkers
	}
	return &PruneRepository{
		planner:       newPrunePlanner(folderRepo, manifestRepo, objectRepo, encryptedRepo),
		objectRepo:    objectRepo,
		encryptedRepo: encryptedRepo,
		locker:        locker,
		workers:       workers,
	}
}

type pruneTarget struct {
	config     *repository.Config
	encrypted  bool
	generation uint64
	plan       *snapshot.PrunePlan
}

type prunePlanner struct {
	folderRepo    FolderRepository
	manifestRepo  ManifestRepository
	objectRepo    ObjectRepository
	encryptedRepo EncryptedObjectRepository
}

func newPrunePlanner(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	encryptedRepo EncryptedObjectRepository,
) prunePlanner {
	return prunePlanner{folderRepo: folderRepo, manifestRepo: manifestRepo, objectRepo: objectRepo, encryptedRepo: encryptedRepo}
}

func (p prunePlanner) plan(ctx context.Context, repositoryPath string) (*pruneTarget, error) {
	conf, err := p.folderRepo.GetConfig(ctx, repositoryPath)
	if err != nil {
		return nil, err
	}

	target := &pruneTarget{config: conf, encrypted: conf.Encryption() != nil}
	if target.encrypted {
		target.generation = conf.Encryption().Generation()
	}

	manifests, err := p.manifestRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	stored, err := p.objectRepo.ListObjects(ctx, target.encrypted, target.generation)
	if err != nil {
		return nil, err
	}

	generations, err := p.encryptedRepo.ListEncryptedGenerations(ctx)
	if err != nil {
		return nil, err
	}

	target.plan = snapshot.NewPrunePlan(manifests, stored)
	target.plan.RecordOrphanGenerations(conf.OrphanGenerations(generations))
	return target, nil
}
