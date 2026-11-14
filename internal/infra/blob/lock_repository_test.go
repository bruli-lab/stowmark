package blob_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/bruli-lab/stowmark/internal/infra/blob/blobtest"
	"github.com/stretchr/testify/require"
)

func newTestLock(t *testing.T, kind repository.LockKind) *repository.Lock {
	t.Helper()
	lock, err := repository.NewLock(kind, repository.NewLockOwner("host", 42), time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	return lock
}

func TestLockRepository(t *testing.T) {
	t.Parallel()

	t.Run(`Given a blob LockRepository with two locks and unrelated blobs,
	when List is called,
	then it returns only the locks with their data`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		store := blobtest.NewMemoryStore()
		repo := blob.NewLockRepository(store)
		shared := newTestLock(t, repository.SharedLock)
		exclusive := newTestLock(t, repository.ExclusiveLock)
		require.NoError(t, repo.Save(ctx, shared))
		require.NoError(t, repo.Save(ctx, exclusive))
		require.NoError(t, store.Put(ctx, "locks/readme.txt", func(io.Writer) error { return nil }))
		require.NoError(t, store.Put(ctx, "locks/nested/lock.json", func(io.Writer) error { return nil }))

		locks, err := repo.List(ctx)

		require.NoError(t, err)
		require.ElementsMatch(t, []repository.Lock{*shared, *exclusive}, locks)
	})

	t.Run(`Given a blob LockRepository with a lock,
	when Delete is called with its id,
	then the lock is no longer listed`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repo := blob.NewLockRepository(blobtest.NewMemoryStore())
		lock := newTestLock(t, repository.ExclusiveLock)
		require.NoError(t, repo.Save(ctx, lock))

		require.NoError(t, repo.Delete(ctx, lock.ID()))

		locks, err := repo.List(ctx)
		require.NoError(t, err)
		require.Empty(t, locks)
	})

	t.Run(`Given a blob LockRepository without locks folder,
	when List is called,
	then it returns no locks`, func(t *testing.T) {
		t.Parallel()

		locks, err := blob.NewLockRepository(blobtest.NewMemoryStore()).List(context.Background())

		require.NoError(t, err)
		require.Empty(t, locks)
	})

	t.Run(`Given a blob LockRepository with a corrupted lock file,
	when List is called,
	then it returns an error instead of ignoring a lock it cannot read`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		store := blobtest.NewMemoryStore()
		require.NoError(t, store.Put(ctx, "locks/broken.json", func(destination io.Writer) error {
			_, err := io.WriteString(destination, "{not json")
			return err
		}))

		_, err := blob.NewLockRepository(store).List(ctx)

		require.Error(t, err)
		require.Contains(t, err.Error(), "locks/broken.json")
	})
}
