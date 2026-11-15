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
	Progress       Progress
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
	codec, err := v.keys.forRepository(ctx, q.RepositoryPath, q.PrivateKeyPath)
	if err != nil {
		return nil, err
	}

	manifest, err := v.manifestRepo.Get(ctx, q.SnapshotID, codec)
	if err != nil {
		return nil, err
	}
	result := snapshot.NewResult(q.SnapshotID)
	items, bytes := contentTotals(manifest.Files())
	progress := startProgress(q.Progress, ProgressVerifying, items, bytes)
	defer progress.Finish()

	var mu sync.Mutex

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(v.workers)

	for _, file := range manifest.Files() {
		if groupCtx.Err() != nil {
			break
		}

		group.Go(func() error {
			reason, err := v.verifyFile(groupCtx, &file, codec, progress)
			if err != nil {
				return err
			}
			progress.Advance(1, 0)

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

func (v *VerifySnapshot) verifyFile(ctx context.Context, file *snapshot.File, codec repository.Codec, progress Progress) (string, error) {
	if !file.HasContent() {
		return "", nil
	}
	chunks := file.Chunks()
	if len(chunks) == 0 {
		reason, err := v.verifier.verify(ctx, file.Hash(), file.Size(), codec)
		if err == nil {
			progress.Advance(0, file.Size())
		}
		return reason, err
	}
	var verified int64
	defer func() { progress.Advance(0, file.Size()-verified) }()
	for index, chunk := range chunks {
		reason, err := v.verifier.verify(ctx, chunk.Hash(), chunk.Size(), codec)
		if err != nil {
			return "", err
		}
		if reason != "" {
			return fmt.Sprintf("chunk %d/%d: %s", index+1, len(chunks), reason), nil
		}
		verified += chunk.Size()
		progress.Advance(0, chunk.Size())
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
