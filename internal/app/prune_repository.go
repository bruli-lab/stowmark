package app

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	PrivateKeyPath *string
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
		target, err := p.planner.plan(ctx, co.RepositoryPath, co.PrivateKeyPath)
		if err != nil {
			return err
		}
		repositoryID := target.config.ID().String()
		if len(target.plan.Missing()) > 0 {
			events = append(events, target.plan.MissingDetected(repositoryID))
		}
		postponed, err := p.deleteObjects(ctx, target)
		if err != nil {
			return err
		}
		if len(target.plan.Unreferenced()) > len(postponed.objects) {
			events = append(events, target.plan.PrunedExcept(repositoryID, postponed.objects))
		}
		if err := p.deleteGenerations(ctx, target, postponed); err != nil {
			return err
		}
		if len(target.plan.OrphanGenerations()) > len(postponed.generations) {
			events = append(events, target.plan.GenerationsRemovedExcept(repositoryID, postponed.generations))
		}
		if postponed.any() {
			events = append(events, target.plan.DeletionPostponed(repositoryID, postponed.objects, postponed.generations, postponed.until))
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

type postponedDeletions struct {
	mu          sync.Mutex
	objects     []string
	generations []uint64
	until       time.Time
}

func (p *postponedDeletions) object(hash string, until time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.objects = append(p.objects, hash)
	p.extend(until)
}

func (p *postponedDeletions) generation(generation uint64, until time.Time) {
	p.generations = append(p.generations, generation)
	p.extend(until)
}

func (p *postponedDeletions) extend(until time.Time) {
	if until.After(p.until) {
		p.until = until
	}
}

func (p *postponedDeletions) any() bool {
	return len(p.objects) > 0 || len(p.generations) > 0
}

func (p *PruneRepository) deleteObjects(ctx context.Context, target *pruneTarget) (*postponedDeletions, error) {
	postponed := &postponedDeletions{}
	hashes := target.plan.Unreferenced()
	if len(hashes) == 0 {
		return postponed, nil
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(min(p.workers, len(hashes)))

	for _, hash := range hashes {
		if groupCtx.Err() != nil {
			break
		}

		group.Go(func() error {
			err := p.objectRepo.DeleteObject(groupCtx, hash, target.encrypted, target.generation)
			if until, retained := repository.RetainedUntil(err); retained {
				postponed.object(hash, until)
				return nil
			}
			if err != nil {
				return fmt.Errorf("prune object %q: %w", hash, err)
			}
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	return postponed, ctx.Err()
}

func (p *PruneRepository) deleteGenerations(ctx context.Context, target *pruneTarget, postponed *postponedDeletions) error {
	for _, generation := range target.plan.OrphanGenerations() {
		err := p.encryptedRepo.DeleteEncryptedGeneration(ctx, generation)
		if until, retained := repository.RetainedUntil(err); retained {
			postponed.generation(generation, until)
			continue
		}
		if err != nil {
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
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *PruneRepository {
	return NewPruneRepositoryWithWorkers(folderRepo, manifestRepo, objectRepo, encryptedRepo, asymmetricRepo, symmetricRepo, locker, defaultPruneWorkers)
}

func NewPruneRepositoryWithWorkers(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	encryptedRepo EncryptedObjectRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
	workers int,
) *PruneRepository {
	if workers <= 0 {
		workers = defaultPruneWorkers
	}
	return &PruneRepository{
		planner:       newPrunePlanner(folderRepo, manifestRepo, objectRepo, encryptedRepo, newKeyring(folderRepo, asymmetricRepo, symmetricRepo)),
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
	keys          keyring
}

func newPrunePlanner(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	encryptedRepo EncryptedObjectRepository,
	keys keyring,
) prunePlanner {
	return prunePlanner{folderRepo: folderRepo, manifestRepo: manifestRepo, objectRepo: objectRepo, encryptedRepo: encryptedRepo, keys: keys}
}

func (p *prunePlanner) plan(ctx context.Context, repositoryPath string, privateKeyPath *string) (*pruneTarget, error) {
	conf, err := p.folderRepo.GetConfig(ctx, repositoryPath)
	if err != nil {
		return nil, err
	}

	target := &pruneTarget{config: conf, encrypted: conf.Encryption() != nil}
	if target.encrypted {
		target.generation = conf.Encryption().Generation()
	}

	codec, err := p.keys.forConfig(ctx, conf, privateKeyPath)
	if err != nil {
		return nil, err
	}

	manifests, err := p.manifestRepo.List(ctx, codec)
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
