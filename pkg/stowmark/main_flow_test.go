//go:build integration

package stowmark_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/bruli-lab/stowmark/internal/infra/blob/blobtest"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/bruli-lab/stowmark/pkg/stowmark"
	"github.com/stretchr/testify/require"
)

type Folders struct {
	Source, Repository,
	keyPairsFolder, newKeyPairsFolder string
	Restore, AlternativeRestore *string
}

func mainFlow(t *testing.T, ctx context.Context, folders Folders, formatVersion int) {
	t.Helper()

	require.NoError(t, os.MkdirAll(folders.Source, 0o755))
	createSourceFixture(t, folders.Source)

	fileExample := fmt.Sprintf("%s/README.txt", folders.Source)

	handler, err := stowmark.NewHandler(ctx, folders.Repository)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, handler.Close())
	})

	keys, err := handler.KeyGenerate(ctx, folders.keyPairsFolder)
	require.NoError(t, err)

	compressionLevel := 3
	err = handler.Init(
		ctx,
		&stowmark.Compression{
			Type:  "zstd",
			Level: &compressionLevel,
		},
		formatVersion,
		new(keys.PublicKey()),
		false,
	)
	require.NoError(t, err)

	store := blobStore(t, ctx, folders.Repository)
	lockRepo := blob.NewLockRepository(store)
	foreignLock, err := repository.NewLock(repository.ExclusiveLock, repository.NewLockOwner("other-host", 1), time.Now())
	require.NoError(t, err)
	require.NoError(t, lockRepo.Save(ctx, foreignLock))
	_, err = handler.CreateSnapshot(ctx, folders.Source, new(keys.PrivateKey()))
	require.ErrorIs(t, err, repository.ErrRepositoryLocked, "a snapshot must not be created while another process holds an exclusive lock")
	require.NoError(t, lockRepo.Delete(ctx, foreignLock.ID()))

	created, err := handler.CreateSnapshot(
		ctx,
		folders.Source,
		new(keys.PrivateKey()),
	)
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)
	require.Equal(t, 3, created.FileCount)

	snapshots, err := handler.ListSnapshots(ctx)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	require.Equal(t, created.ID, snapshots[0].ID)

	snapshot, err := handler.GetManifest(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, snapshot.ID)
	require.Len(t, snapshot.Files, created.FileCount)

	verification, err := handler.VerifySnapshot(
		ctx,
		created.ID,
		new(keys.PrivateKey()),
	)
	require.NoError(t, err)
	require.True(t, verification.IsSuccess)
	require.Empty(t, verification.Failed)

	restored, err := handler.RestoreSnapshot(
		ctx,
		created.ID,
		folders.Restore,
		new(keys.PrivateKey()),
	)
	require.NoError(t, err)
	require.True(t, restored.IsSuccess)
	require.Empty(t, restored.Failed)

	requireDirectoriesEqual(t, folders.Source, *folders.Restore)

	err = handler.RestoreFile(ctx, created.ID, fileExample, folders.AlternativeRestore, new(keys.PrivateKey()))
	require.NoError(t, err)
	requireFileExists(t, fileExample)

	newKeys, err := handler.KeyGenerate(ctx, folders.newKeyPairsFolder)
	require.NoError(t, err)
	require.NotEqual(t, keys.PublicKey(), newKeys.PublicKey())

	err = handler.Rewrap(ctx, keys.PrivateKey(), newKeys.PublicKey())
	require.NoError(t, err)

	err = handler.ReKey(ctx, newKeys.PrivateKey(), newKeys.PublicKey())
	require.NoError(t, err)

	rekeyed, err := handler.VerifySnapshot(ctx, created.ID, new(newKeys.PrivateKey()))
	require.NoError(t, err)
	require.True(t, rekeyed.IsSuccess)
	require.Empty(t, rekeyed.Failed)

	rekeyedRestore := filepath.Join(t.TempDir(), "rekeyed-restore")
	restored, err = handler.RestoreSnapshot(ctx, created.ID, &rekeyedRestore, new(newKeys.PrivateKey()))
	require.NoError(t, err)
	require.True(t, restored.IsSuccess)
	requireDirectoriesEqual(t, folders.Source, rekeyedRestore)

	encryptedKeys, err := store.List(ctx, "objects/encrypted")
	require.NoError(t, err)
	require.NotEmpty(t, encryptedKeys)
	generations := map[string]struct{}{}
	for _, key := range encryptedKeys {
		generations[strings.SplitN(key, "/", 4)[2]] = struct{}{}
	}
	require.Len(t, generations, 1, "the previous encrypted generation must be deleted after rekey")

	remainingLocks, err := store.List(ctx, repository.LocksFolder)
	require.NoError(t, err)
	require.Empty(t, remainingLocks, "every operation must release its repository lock")

	staleLock, err := repository.NewLock(repository.ExclusiveLock, repository.NewLockOwner("crashed-host", 2), time.Now().Add(-repository.LockStaleAfter-time.Minute))
	require.NoError(t, err)
	activeLock, err := repository.NewLock(repository.SharedLock, repository.NewLockOwner("other-host", 3), time.Now())
	require.NoError(t, err)
	require.NoError(t, lockRepo.Save(ctx, staleLock))
	require.NoError(t, lockRepo.Save(ctx, activeLock))
	unlocked, err := handler.Unlock(ctx, false)
	require.NoError(t, err)
	require.Equal(t, stowmark.UnlockResult{RemovedLocks: 1, ActiveLocks: 1}, *unlocked)
	unlocked, err = handler.Unlock(ctx, true)
	require.NoError(t, err)
	require.Equal(t, stowmark.UnlockResult{RemovedLocks: 1, ActiveLocks: 0}, *unlocked)
	remainingLocks, err = store.List(ctx, repository.LocksFolder)
	require.NoError(t, err)
	require.Empty(t, remainingLocks)

	require.NoError(t, os.Remove(filepath.Join(folders.Source, "documents", "notes.txt")))
	writeFixtureFile(t, folders.Source, "documents/new.txt", []byte("A file added after the first snapshot.\n"))
	writeFixtureFile(t, folders.Source, stowmark.IgnoreFileName, []byte("# temporary files\n*.tmp\n"))
	writeFixtureFile(t, folders.Source, "documents/scratch.tmp", []byte("not backed up\n"))
	writeFixtureFile(t, folders.Source, "cache/data.bin", []byte("not backed up either\n"))
	second, err := handler.CreateSnapshot(ctx, folders.Source, new(newKeys.PrivateKey()), "cache/")
	require.NoError(t, err)
	secondManifest, err := handler.GetManifest(ctx, second.ID)
	require.NoError(t, err)
	for _, file := range secondManifest.Files {
		require.NotContains(t, file.Path, "scratch.tmp", "files matching the ignore file must not be backed up")
		require.NotContains(t, file.Path, "cache", "files matching an exclude pattern must not be backed up")
	}
	require.NoError(t, os.Remove(filepath.Join(folders.Source, "documents", "scratch.tmp")))
	require.NoError(t, os.RemoveAll(filepath.Join(folders.Source, "cache")))

	keepLast := stowmark.RetentionPolicy{KeepLast: 1}
	plan, err := handler.PlanForget(ctx, keepLast)
	require.NoError(t, err)
	require.Len(t, plan, 2)
	require.Equal(t, second.ID, plan[0].ID)
	require.True(t, plan[0].Keep)
	require.Equal(t, []string{"last"}, plan[0].Reasons)
	require.False(t, plan[1].Keep)
	snapshots, err = handler.ListSnapshots(ctx)
	require.NoError(t, err)
	require.Len(t, snapshots, 2, "planning must not forget snapshots")

	prunePlan, err := handler.PlanPrune(ctx)
	require.NoError(t, err)
	require.Zero(t, prunePlan.UnreferencedObjects)
	require.Zero(t, prunePlan.MissingObjects)

	forgotten, err := handler.Forget(ctx, keepLast)
	require.NoError(t, err)
	require.Len(t, forgotten, 1)
	require.Equal(t, created.ID, forgotten[0].ID)
	snapshots, err = handler.ListSnapshots(ctx)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	require.Equal(t, second.ID, snapshots[0].ID)

	prunePlan, err = handler.PlanPrune(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, prunePlan.UnreferencedObjects, "only the object of the removed file must be unreferenced")
	require.Equal(t, prunePlan.ReferencedObjects, prunePlan.StoredObjects-prunePlan.UnreferencedObjects)

	pruned, err := handler.Prune(ctx)
	require.NoError(t, err)
	require.Equal(t, stowmark.PruneResult{RemovedObjects: 1}, *pruned)

	pruned, err = handler.Prune(ctx)
	require.NoError(t, err)
	require.Equal(t, stowmark.PruneResult{}, *pruned, "a second prune must find nothing to remove")

	verification, err = handler.VerifySnapshot(ctx, second.ID, new(newKeys.PrivateKey()))
	require.NoError(t, err)
	require.True(t, verification.IsSuccess)
	prunedRestore := filepath.Join(t.TempDir(), "pruned-restore")
	restored, err = handler.RestoreSnapshot(ctx, second.ID, &prunedRestore, new(newKeys.PrivateKey()))
	require.NoError(t, err)
	require.True(t, restored.IsSuccess)
	requireDirectoriesEqual(t, folders.Source, prunedRestore)

	checked, err := handler.Check(ctx, stowmark.CheckOptions{})
	require.NoError(t, err)
	require.False(t, checked.HasErrors)
	require.False(t, checked.ReadData)
	require.Equal(t, 1, checked.Snapshots)
	require.Zero(t, checked.UnreferencedObjects)

	checked, err = handler.Check(ctx, stowmark.CheckOptions{ReadData: true, PrivateKey: new(newKeys.PrivateKey())})
	require.NoError(t, err)
	require.False(t, checked.HasErrors)
	require.Equal(t, checked.ReferencedObjects, checked.ReadObjects)
	require.Empty(t, checked.CorruptObjects)

	objectKeys, err := store.List(ctx, "objects/encrypted")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(objectKeys), 2)
	missingKey, corruptKey := objectKeys[0], objectKeys[1]
	require.NoError(t, store.Delete(ctx, missingKey))
	require.NoError(t, store.Delete(ctx, corruptKey))
	require.NoError(t, store.Put(ctx, corruptKey, func(w io.Writer) error {
		_, err := w.Write([]byte("this is not an encrypted object"))
		return err
	}))

	checked, err = handler.Check(ctx, stowmark.CheckOptions{ReadData: true, PrivateKey: new(newKeys.PrivateKey())})
	require.NoError(t, err)
	require.True(t, checked.HasErrors)
	require.Len(t, checked.MissingObjects, 1)
	require.Equal(t, objectHash(missingKey), checked.MissingObjects[0].Hash)
	require.Len(t, checked.CorruptObjects, 1)
	require.Equal(t, objectHash(corruptKey), checked.CorruptObjects[0].ID)

	remainingLocks, err = store.List(ctx, repository.LocksFolder)
	require.NoError(t, err)
	require.Empty(t, remainingLocks)

	var contractRepositories atomic.Int64
	t.Run("blob store contract", func(t *testing.T) {
		blobtest.RunStoreContract(t, func(t *testing.T) blob.Store {
			return blobStore(t, ctx, fmt.Sprintf("%s-contract-%d", folders.Repository, contractRepositories.Add(1)))
		})
	})
}

func blobStore(t *testing.T, ctx context.Context, repositoryURL string) blob.Store {
	t.Helper()

	repos, err := repositories.NewHandler(ctx, repositoryURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = repos.Close()
	})
	store, err := repos.BlobStore()
	require.NoError(t, err)

	return store
}

func objectHash(key string) string {
	segments := strings.Split(key, "/")
	return segments[len(segments)-2] + segments[len(segments)-1]
}
