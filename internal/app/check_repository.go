//nolint:dupl // CQS query handlers intentionally share the same adapter structure.
package app

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"golang.org/x/sync/errgroup"
)

const CheckRepositoryQueryName = "check-repository"

var ErrPrivateKeyRequiredToReadData = errors.New("the private key is required to read the data of an encrypted repository")

type CheckRepositoryQuery struct {
	RepositoryPath string
	ReadData       bool
	Subset         *snapshot.DataSubset
	PrivateKeyPath *string
}

func (c CheckRepositoryQuery) Name() string {
	return CheckRepositoryQueryName
}

func (c CheckRepositoryQuery) readsData() bool {
	return c.ReadData || c.Subset != nil
}

type CheckRepository struct {
	folderRepo    FolderRepository
	manifestRepo  ManifestRepository
	objectRepo    ObjectRepository
	encryptedRepo EncryptedObjectRepository
	lockRepo      LockRepository
	keys          keyring
	verifier      objectVerifier
	locker        *RepositoryLocker
	workers       int
}

func (c *CheckRepository) Handle(ctx context.Context, query cqs.Query) (any, error) {
	q, ok := query.(CheckRepositoryQuery)
	if !ok {
		return nil, cqs.NewInvalidQueryError(CheckRepositoryQueryName, query.Name())
	}

	report, err := withLockedResult(ctx, c.locker, repository.SharedLock, func(ctx context.Context) (*snapshot.CheckReport, error) {
		return c.check(ctx, q)
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (c *CheckRepository) check(ctx context.Context, q CheckRepositoryQuery) (*snapshot.CheckReport, error) {
	conf, err := c.folderRepo.GetConfig(ctx, q.RepositoryPath)
	if err != nil {
		return nil, err
	}
	encryptionConf := conf.Encryption()
	encrypted := encryptionConf != nil
	var generation uint64
	if encrypted {
		generation = encryptionConf.Generation()
	}
	if q.readsData() && encrypted && q.PrivateKeyPath == nil {
		return nil, ErrPrivateKeyRequiredToReadData
	}

	manifests, corrupt, err := c.readManifests(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := c.objectRepo.ListObjects(ctx, encrypted, generation)
	if err != nil {
		return nil, err
	}
	report := snapshot.NewCheckReport(manifests, corrupt, stored)

	if err := c.checkLocks(ctx, report); err != nil {
		return nil, err
	}
	if err := c.checkGenerations(ctx, report, conf); err != nil {
		return nil, err
	}
	if !q.readsData() {
		return report, nil
	}

	key := objectKey{}
	if encrypted {
		key, err = c.keys.forConfig(ctx, encryptionConf, q.PrivateKeyPath)
		if err != nil {
			return nil, err
		}
	}
	hashes := report.ReadableObjects()
	if q.Subset != nil {
		hashes = q.Subset.Select(hashes, rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
	}
	problems, err := c.readData(ctx, report, hashes, key)
	if err != nil {
		return nil, err
	}
	report.RecordData(len(hashes), problems)

	return report, nil
}

func (c *CheckRepository) readManifests(ctx context.Context) ([]snapshot.Manifest, []snapshot.ManifestProblem, error) {
	ids, err := c.manifestRepo.ListIDs(ctx)
	if err != nil {
		return nil, nil, err
	}
	manifests := make([]snapshot.Manifest, 0, len(ids))
	corrupt := make([]snapshot.ManifestProblem, 0)
	for _, id := range ids {
		manifest, err := c.manifestRepo.Get(ctx, id)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, ctxErr
		}
		if err != nil {
			corrupt = append(corrupt, snapshot.NewManifestProblem(id, err.Error()))
			continue
		}
		manifests = append(manifests, *manifest)
	}
	return manifests, corrupt, nil
}

func (c *CheckRepository) checkLocks(ctx context.Context, report *snapshot.CheckReport) error {
	locks, err := c.lockRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("list repository locks: %w", err)
	}
	now := time.Now()
	stale := 0
	for i := range locks {
		if locks[i].IsStale(now) {
			stale++
		}
	}
	report.RecordStaleLocks(stale)
	return nil
}

func (c *CheckRepository) checkGenerations(ctx context.Context, report *snapshot.CheckReport, conf *repository.Config) error {
	generations, err := c.encryptedRepo.ListEncryptedGenerations(ctx)
	if err != nil {
		return err
	}
	report.RecordOrphanGenerations(conf.OrphanGenerations(generations))
	return nil
}

func (c *CheckRepository) readData(ctx context.Context, report *snapshot.CheckReport, hashes []string, key objectKey) ([]snapshot.ObjectProblem, error) {
	var (
		mu       sync.Mutex
		problems = make([]snapshot.ObjectProblem, 0)
	)
	if len(hashes) == 0 {
		return problems, nil
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(min(c.workers, len(hashes)))

	for _, hash := range hashes {
		if groupCtx.Err() != nil {
			break
		}

		group.Go(func() error {
			reason, err := c.verifier.verify(groupCtx, hash, report.ObjectSize(hash), key)
			if err != nil {
				return err
			}
			if reason != "" {
				mu.Lock()
				defer mu.Unlock()
				problems = append(problems, snapshot.NewObjectProblem(hash, reason))
			}
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	return problems, ctx.Err()
}

func NewCheckRepository(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	encryptedRepo EncryptedObjectRepository,
	lockRepo LockRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *CheckRepository {
	return NewCheckRepositoryWithWorkers(folderRepo, manifestRepo, objectRepo, encryptedRepo, lockRepo, asymmetricRepo, symmetricRepo, locker, defaultVerifyWorkers)
}

func NewCheckRepositoryWithWorkers(
	folderRepo FolderRepository,
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	encryptedRepo EncryptedObjectRepository,
	lockRepo LockRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
	workers int,
) *CheckRepository {
	if workers <= 0 {
		workers = defaultVerifyWorkers
	}
	return &CheckRepository{
		folderRepo:    folderRepo,
		manifestRepo:  manifestRepo,
		objectRepo:    objectRepo,
		encryptedRepo: encryptedRepo,
		lockRepo:      lockRepo,
		keys:          newKeyring(folderRepo, asymmetricRepo, symmetricRepo),
		verifier:      newObjectVerifier(objectRepo),
		locker:        locker,
		workers:       workers,
	}
}
