package gcs_test

import (
	"io"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/bruli-lab/stowmark/internal/infra/gcs"
	"github.com/stretchr/testify/require"
)

const (
	lockBucket = "backups"
	lockRoot   = "repo"
)

var lockedBucket = fakeBucket{retention: true, versioning: true}

func retentionFor(t *testing.T, mode repository.ObjectLockMode, period time.Duration) repository.Retention {
	t.Helper()
	lock, err := repository.NewObjectLock(mode, period)
	require.NoError(t, err)
	return lock.RetentionFrom(time.Now())
}

func writeData(data string) func(io.Writer) error {
	return func(w io.Writer) error {
		_, err := io.WriteString(w, data)
		return err
	}
}

func TestBlobStore_VerifyObjectLock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		bucket      fakeBucket
		expectedErr error
	}{
		{name: "with object retention and versioning, then it returns nil", bucket: lockedBucket},
		{name: "without object retention, then it returns a not enabled error", bucket: fakeBucket{versioning: true}, expectedErr: gcs.ErrObjectRetentionNotEnabled},
		{name: "without versioning, then it returns a versioning error", bucket: fakeBucket{retention: true}, expectedErr: gcs.ErrVersioningNotEnabled},
		{name: "with a bucket retention policy, then it returns a retention policy error", bucket: fakeBucket{retention: true, versioning: true, policy: time.Hour}, expectedErr: gcs.ErrBucketRetentionPolicy},
	}
	for _, tt := range tests {
		t.Run(`Given a GCS bucket,
		when VerifyObjectLock is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			_, client := newFakeGCS(t, lockBucket, tt.bucket)
			store := gcs.NewBlobStore(client, lockBucket, lockRoot)

			err := store.VerifyObjectLock(t.Context())

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestBlobStore_Retention(t *testing.T) {
	t.Parallel()

	t.Run(`Given an object written with a compliance retention,
	when RetainedUntil is called,
	then it returns the retain until date and the object uses the locked mode`, func(t *testing.T) {
		t.Parallel()

		fake, client := newFakeGCS(t, lockBucket, lockedBucket)
		store := gcs.NewBlobStore(client, lockBucket, lockRoot)
		retention := retentionFor(t, repository.ComplianceObjectLock, time.Hour)
		require.NoError(t, store.PutRetained(t.Context(), "objects/ab/cdef", retention, writeData("data")))

		until, err := store.RetainedUntil(t.Context(), "objects/ab/cdef")

		require.NoError(t, err)
		require.WithinDuration(t, retention.Until(), until, time.Millisecond)
		require.Equal(t, "Locked", fake.objects[lockRoot+"/objects/ab/cdef"][0].retention.Mode)
	})

	t.Run(`Given an object without retention,
	when RetainedUntil is called,
	then it returns a zero date`, func(t *testing.T) {
		t.Parallel()

		_, client := newFakeGCS(t, lockBucket, lockedBucket)
		store := gcs.NewBlobStore(client, lockBucket, lockRoot)
		require.NoError(t, store.Put(t.Context(), "objects/ab/cdef", writeData("data")))

		until, err := store.RetainedUntil(t.Context(), "objects/ab/cdef")

		require.NoError(t, err)
		require.True(t, until.IsZero())
	})

	t.Run(`Given an object that does not exist,
	when RetainedUntil and Retain are called,
	then they return a not found error`, func(t *testing.T) {
		t.Parallel()

		_, client := newFakeGCS(t, lockBucket, lockedBucket)
		store := gcs.NewBlobStore(client, lockBucket, lockRoot)

		_, err := store.RetainedUntil(t.Context(), "objects/ab/cdef")
		require.ErrorIs(t, err, blob.ErrNotFound)
		err = store.Retain(t.Context(), "objects/ab/cdef", retentionFor(t, repository.GovernanceObjectLock, time.Hour))
		require.ErrorIs(t, err, blob.ErrNotFound)
	})

	t.Run(`Given a retained object,
	when Retain is called with a later date,
	then the retention is extended`, func(t *testing.T) {
		t.Parallel()

		_, client := newFakeGCS(t, lockBucket, lockedBucket)
		store := gcs.NewBlobStore(client, lockBucket, lockRoot)
		require.NoError(t, store.PutRetained(t.Context(), "snapshots/id.json", retentionFor(t, repository.GovernanceObjectLock, time.Hour), writeData("{}")))
		extended := retentionFor(t, repository.GovernanceObjectLock, 2*time.Hour)

		require.NoError(t, store.Retain(t.Context(), "snapshots/id.json", extended))

		until, err := store.RetainedUntil(t.Context(), "snapshots/id.json")
		require.NoError(t, err)
		require.WithinDuration(t, extended.Until(), until, time.Millisecond)
	})
}

func TestBlobStore_DeleteWithObjectLock(t *testing.T) {
	t.Parallel()

	t.Run(`Given a bucket with object retention and a retained object,
	when Delete is called,
	then it returns a retained error with the retain until date and keeps the object`, func(t *testing.T) {
		t.Parallel()

		fake, client := newFakeGCS(t, lockBucket, lockedBucket)
		store := gcs.NewBlobStore(client, lockBucket, lockRoot)
		retention := retentionFor(t, repository.GovernanceObjectLock, time.Hour)
		require.NoError(t, store.PutRetained(t.Context(), "objects/ab/cdef", retention, writeData("data")))

		err := store.Delete(t.Context(), "objects/ab/cdef")

		until, ok := repository.RetainedUntil(err)
		require.True(t, ok)
		require.WithinDuration(t, retention.Until(), until, time.Millisecond)
		require.Len(t, fake.generations(lockRoot+"/objects/ab/cdef"), 1)
		exists, err := store.Exists(t.Context(), "objects/ab/cdef")
		require.NoError(t, err)
		require.True(t, exists)
	})

	t.Run(`Given a bucket with object retention and an object rewritten twice without retention,
	when Delete is called,
	then it deletes every generation`, func(t *testing.T) {
		t.Parallel()

		fake, client := newFakeGCS(t, lockBucket, lockedBucket)
		store := gcs.NewBlobStore(client, lockBucket, lockRoot)
		require.NoError(t, store.Put(t.Context(), "locks/id.json", writeData("one")))
		require.NoError(t, store.Delete(t.Context(), "locks/id.json"))
		require.NoError(t, store.Put(t.Context(), "locks/id.json", writeData("two")))
		require.NoError(t, store.Put(t.Context(), "locks/id.jsonx", writeData("other")))

		require.NoError(t, store.Delete(t.Context(), "locks/id.json"))

		require.Empty(t, fake.generations(lockRoot+"/locks/id.json"))
		require.Len(t, fake.generations(lockRoot+"/locks/id.jsonx"), 1)
	})

	t.Run(`Given a bucket with object retention, a retained object and an unretained one under a prefix,
	when DeletePrefix is called,
	then it deletes the unretained one and returns a retained error`, func(t *testing.T) {
		t.Parallel()

		fake, client := newFakeGCS(t, lockBucket, lockedBucket)
		store := gcs.NewBlobStore(client, lockBucket, lockRoot)
		retention := retentionFor(t, repository.GovernanceObjectLock, time.Hour)
		require.NoError(t, store.PutRetained(t.Context(), "objects/encrypted/1/ab/cdef", retention, writeData("retained")))
		require.NoError(t, store.Put(t.Context(), "objects/encrypted/1/ab/0123", writeData("free")))

		err := store.DeletePrefix(t.Context(), "objects/encrypted/1")

		until, ok := repository.RetainedUntil(err)
		require.True(t, ok)
		require.WithinDuration(t, retention.Until(), until, time.Millisecond)
		require.Empty(t, fake.generations(lockRoot+"/objects/encrypted/1/ab/0123"))
		require.Len(t, fake.generations(lockRoot+"/objects/encrypted/1/ab/cdef"), 1)
	})

	t.Run(`Given a bucket without object retention,
	when Delete is called,
	then it deletes the live object`, func(t *testing.T) {
		t.Parallel()

		fake, client := newFakeGCS(t, lockBucket, fakeBucket{})
		store := gcs.NewBlobStore(client, lockBucket, lockRoot)
		require.NoError(t, store.Put(t.Context(), "objects/ab/cdef", writeData("data")))

		require.NoError(t, store.Delete(t.Context(), "objects/ab/cdef"))

		require.Empty(t, fake.generations(lockRoot+"/objects/ab/cdef"))
	})
}

func TestFolderRepositoryRepository_CreateConfigWithObjectLock(t *testing.T) {
	t.Parallel()

	t.Run(`Given a config with object lock in a bucket with object retention,
	when CreateConfig is called twice,
	then the config is retained and the second write creates a new generation`, func(t *testing.T) {
		t.Parallel()

		fake, client := newFakeGCS(t, lockBucket, lockedBucket)
		lock, err := repository.NewObjectLock(repository.GovernanceObjectLock, time.Hour)
		require.NoError(t, err)
		base := fixtures.ConfigBuilder{}.Build(t)
		conf := base.WithObjectLock(lock)
		folders := gcs.NewFolderRepositoryRepository(client, lockBucket)
		store := gcs.NewBlobStore(client, lockBucket, lockRoot)

		require.NoError(t, folders.CreateConfig(t.Context(), lockRoot, conf))
		require.NoError(t, folders.CreateConfig(t.Context(), lockRoot, conf))

		until, err := store.RetainedUntil(t.Context(), "config.json")
		require.NoError(t, err)
		require.WithinDuration(t, time.Now().Add(time.Hour), until, time.Minute)
		require.Len(t, fake.generations(lockRoot+"/config.json"), 2)
		got, err := folders.GetConfig(t.Context(), lockRoot)
		require.NoError(t, err)
		require.Equal(t, lock, got.ObjectLock())
	})
}
