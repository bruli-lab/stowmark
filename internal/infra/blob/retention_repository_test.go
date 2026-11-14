package blob_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/bruli-lab/stowmark/internal/infra/blob/blobtest"
	"github.com/stretchr/testify/require"
)

const retainedHash = "ab0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcd"

type retainingStore struct {
	*blobtest.MemoryStore
	mu        sync.Mutex
	verifyErr error
	retained  map[string]repository.Retention
}

func newRetainingStore() *retainingStore {
	return &retainingStore{MemoryStore: blobtest.NewMemoryStore(), retained: map[string]repository.Retention{}}
}

func (r *retainingStore) VerifyObjectLock(context.Context) error {
	return r.verifyErr
}

func (r *retainingStore) PutRetained(ctx context.Context, key string, retention repository.Retention, write func(io.Writer) error) error {
	if err := r.Put(ctx, key, write); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retained[key] = retention
	return nil
}

func (r *retainingStore) RetainedUntil(ctx context.Context, key string) (time.Time, error) {
	if exists, _ := r.Exists(ctx, key); !exists {
		return time.Time{}, fmt.Errorf("%w: %q", blob.ErrNotFound, key)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.retained[key].Until(), nil
}

func (r *retainingStore) Retain(ctx context.Context, key string, retention repository.Retention) error {
	if exists, _ := r.Exists(ctx, key); !exists {
		return fmt.Errorf("%w: %q", blob.ErrNotFound, key)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retained[key] = retention
	return nil
}

func (r *retainingStore) retentionOf(key string) (repository.Retention, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	retention, ok := r.retained[key]
	return retention, ok
}

func putBlob(t *testing.T, store blob.Store, key string) {
	t.Helper()
	require.NoError(t, store.Put(t.Context(), key, func(w io.Writer) error {
		_, err := io.Copy(w, strings.NewReader("data"))
		return err
	}))
}

func TestRetentionRepository(t *testing.T) {
	t.Parallel()

	until := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	retention := repository.NewRetention(repository.ComplianceObjectLock, until)
	tests := []struct {
		name string
		item repository.RetainedItem
		key  string
	}{
		{name: "for a plain object", item: repository.NewRetainedObject(retainedHash, false, 0), key: "objects/ab/" + retainedHash[2:]},
		{name: "for an encrypted object", item: repository.NewRetainedObject(retainedHash, true, 4), key: "objects/encrypted/4/ab/" + retainedHash[2:]},
		{name: "for a manifest", item: repository.NewRetainedManifest("20260101T000000Z-abcdef01"), key: "snapshots/20260101T000000Z-abcdef01.json"},
		{name: "for the config", item: repository.NewRetainedConfig(), key: "config.json"},
	}
	for _, tt := range tests {
		t.Run(`Given a store with object lock,
		when Retain and RetainedUntil are called `+tt.name+`,
		then they use the key of the item`, func(t *testing.T) {
			t.Parallel()

			store := newRetainingStore()
			putBlob(t, store, tt.key)
			repo := blob.NewRetentionRepository(store)

			require.NoError(t, repo.Retain(t.Context(), tt.item, retention))
			got, err := repo.RetainedUntil(t.Context(), tt.item)

			require.NoError(t, err)
			require.Equal(t, until, got)
			stored, ok := store.retentionOf(tt.key)
			require.True(t, ok)
			require.Equal(t, repository.ComplianceObjectLock, stored.Mode())
		})
	}

	t.Run(`Given a store with object lock without the item,
	when Retain and RetainedUntil are called,
	then they return a not found error`, func(t *testing.T) {
		t.Parallel()

		repo := blob.NewRetentionRepository(newRetainingStore())

		_, err := repo.RetainedUntil(t.Context(), repository.NewRetainedManifest("missing"))
		require.ErrorAs(t, err, new(*repository.NotFoundError))
		err = repo.Retain(t.Context(), repository.NewRetainedObject(retainedHash, false, 0), retention)
		require.ErrorAs(t, err, new(*repository.NotFoundError))
	})

	t.Run(`Given a store with object lock,
	when VerifyObjectLock is called,
	then it returns the result of the store`, func(t *testing.T) {
		t.Parallel()

		store := newRetainingStore()
		store.verifyErr = repository.ErrObjectLockDisabled

		err := blob.NewRetentionRepository(store).VerifyObjectLock(t.Context())

		require.ErrorIs(t, err, repository.ErrObjectLockDisabled)
	})

	t.Run(`Given a store without object lock,
	when any retention operation is called,
	then it returns object lock unsupported error`, func(t *testing.T) {
		t.Parallel()

		repo := blob.NewRetentionRepository(blobtest.NewMemoryStore())

		require.ErrorIs(t, repo.VerifyObjectLock(t.Context()), repository.ErrObjectLockUnsupported)
		_, err := repo.RetainedUntil(t.Context(), repository.NewRetainedConfig())
		require.ErrorIs(t, err, repository.ErrObjectLockUnsupported)
		require.ErrorIs(t, repo.Retain(t.Context(), repository.NewRetainedConfig(), retention), repository.ErrObjectLockUnsupported)
	})

	t.Run(`Given a store with object lock and an invalid object hash,
	when RetainedUntil is called,
	then it returns invalid hash error`, func(t *testing.T) {
		t.Parallel()

		_, err := blob.NewRetentionRepository(newRetainingStore()).RetainedUntil(t.Context(), repository.NewRetainedObject("zz", false, 0))

		require.ErrorIs(t, err, blob.ErrInvalidHash)
	})
}

func TestManifestRepository_SaveWithObjectLock(t *testing.T) {
	t.Parallel()

	lock, err := repository.NewObjectLock(repository.GovernanceObjectLock, 24*time.Hour)
	require.NoError(t, err)
	locked := plainCodec.WithObjectLock(lock)
	manifest := manifestAt(t, "20260101T000000Z-abcdef01", time.Now())

	t.Run(`Given a store with object lock and a codec with object lock,
	when Save is called,
	then it writes the manifest with a retention of one period`, func(t *testing.T) {
		t.Parallel()

		store := newRetainingStore()
		before := time.Now()

		require.NoError(t, blob.NewManifestRepository(store).Save(t.Context(), manifest, locked))

		retention, ok := store.retentionOf("snapshots/20260101T000000Z-abcdef01.json")
		require.True(t, ok)
		require.Equal(t, repository.GovernanceObjectLock, retention.Mode())
		require.WithinDuration(t, before.Add(lock.Period()), retention.Until(), 2*time.Second)
	})

	t.Run(`Given a store without object lock and a codec with object lock,
	when Save is called,
	then it returns object lock unsupported error and writes nothing`, func(t *testing.T) {
		t.Parallel()

		store := blobtest.NewMemoryStore()

		err := blob.NewManifestRepository(store).Save(t.Context(), manifest, locked)

		require.ErrorIs(t, err, repository.ErrObjectLockUnsupported)
		require.Empty(t, store.Keys())
	})

	t.Run(`Given a store with object lock and a codec without object lock,
	when Save is called,
	then it writes the manifest without retention`, func(t *testing.T) {
		t.Parallel()

		store := newRetainingStore()

		require.NoError(t, blob.NewManifestRepository(store).Save(t.Context(), manifest, plainCodec))

		_, ok := store.retentionOf("snapshots/20260101T000000Z-abcdef01.json")
		require.False(t, ok)
		require.Len(t, store.Keys(), 1)
	})
}
