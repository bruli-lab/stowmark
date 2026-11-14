//nolint:dupl // CQS query handlers intentionally share the same adapter structure.
package app

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"golang.org/x/sync/errgroup"
)

const VerifySnapshotQueryName = "verify-snapshot"

var defaultVerifyWorkers = runtime.GOMAXPROCS(0) * 4

type VerifySnapshotQuery struct {
	SnapshotID     string
	RepositoryPath string
	PrivateKeyPath *string
}

func (v VerifySnapshotQuery) Name() string {
	return VerifySnapshotQueryName
}

type VerifySnapshot struct {
	manifestRepo ManifestRepository
	verifier     objectVerifier
	keys         keyring
	locker       *RepositoryLocker
	workers      int
}

func (v *VerifySnapshot) Handle(ctx context.Context, query cqs.Query) (any, error) {
	q, ok := query.(VerifySnapshotQuery)
	if !ok {
		return nil, cqs.NewInvalidQueryError(VerifySnapshotQueryName, query.Name())
	}
	result, err := withLockedResult(ctx, v.locker, repository.SharedLock, func(ctx context.Context) (*snapshot.Result, error) {
		return v.verify(ctx, q)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (v *VerifySnapshot) verify(ctx context.Context, q VerifySnapshotQuery) (*snapshot.Result, error) {
	key, err := v.keys.forRepository(ctx, q.RepositoryPath, q.PrivateKeyPath)
	if err != nil {
		return nil, err
	}

	manifest, err := v.manifestRepo.Get(ctx, q.SnapshotID)
	if err != nil {
		return nil, err
	}
	result := snapshot.NewResult(q.SnapshotID)

	var mu sync.Mutex

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(v.workers)

	for _, file := range manifest.Files() {
		if groupCtx.Err() != nil {
			break
		}

		group.Go(func() error {
			reason, err := v.verifyFile(groupCtx, &file, key)
			if err != nil {
				return err
			}

			mu.Lock()
			defer mu.Unlock()

			if reason != "" {
				result.AddFailed(*snapshot.NewFailedResult(file.Path(), reason))
				return nil
			}

			result.AddSuccess()

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (v *VerifySnapshot) verifyFile(ctx context.Context, file *snapshot.File, key objectKey) (string, error) {
	chunks := file.Chunks()
	if len(chunks) == 0 {
		return v.verifier.verify(ctx, file.Hash(), file.Size(), key)
	}
	for index, chunk := range chunks {
		reason, err := v.verifier.verify(ctx, chunk.Hash(), chunk.Size(), key)
		if err != nil {
			return "", err
		}
		if reason != "" {
			return fmt.Sprintf("chunk %d/%d: %s", index+1, len(chunks), reason), nil
		}
	}

	return "", nil
}

func NewVerifySnapshot(
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	folderRepo FolderRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
) *VerifySnapshot {
	return NewVerifySnapshotWithWorkers(manifestRepo, objectRepo, folderRepo, asymmetricRepo, symmetricRepo, locker, defaultVerifyWorkers)
}

func NewVerifySnapshotWithWorkers(
	manifestRepo ManifestRepository,
	objectRepo ObjectRepository,
	folderRepo FolderRepository,
	asymmetricRepo AsymmetricKeyPairRepository,
	symmetricRepo SymmetricKeyRepository,
	locker *RepositoryLocker,
	workers int,
) *VerifySnapshot {
	if workers <= 0 {
		workers = defaultVerifyWorkers
	}
	return &VerifySnapshot{
		manifestRepo: manifestRepo,
		verifier:     newObjectVerifier(objectRepo),
		keys:         newKeyring(folderRepo, asymmetricRepo, symmetricRepo),
		locker:       locker,
		workers:      workers,
	}
}
