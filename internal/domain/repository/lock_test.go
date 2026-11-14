package repository_test

import (
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var lockOwner = repository.NewLockOwner("host", 42)

func newLock(t *testing.T, kind repository.LockKind, createdAt time.Time) *repository.Lock {
	t.Helper()
	lock, err := repository.NewLock(kind, lockOwner, createdAt)
	require.NoError(t, err)
	return lock
}

func TestParseLockKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		value        string
		expectedKind repository.LockKind
		expectedErr  error
	}{
		{name: "with shared, then it returns a shared lock kind", value: "shared", expectedKind: repository.SharedLock},
		{name: "with exclusive, then it returns an exclusive lock kind", value: "exclusive", expectedKind: repository.ExclusiveLock},
		{name: "with an unknown value, then it returns an invalid lock kind error", value: "other", expectedErr: repository.ErrInvalidLockKind},
	}
	for _, tt := range tests {
		t.Run(`Given a lock kind value,
		when ParseLockKind is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			kind, err := repository.ParseLockKind(tt.value)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedKind, kind)
		})
	}
}

func TestHydrateLock(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name        string
		id          uuid.UUID
		kind        repository.LockKind
		owner       repository.LockOwner
		createdAt   time.Time
		expectedErr error
	}{
		{name: "with an empty id, then it returns an invalid lock id error", kind: repository.SharedLock, owner: lockOwner, createdAt: now, expectedErr: repository.ErrInvalidLockID},
		{name: "with an unknown kind, then it returns an invalid lock kind error", id: uuid.New(), kind: "other", owner: lockOwner, createdAt: now, expectedErr: repository.ErrInvalidLockKind},
		{name: "without owner, then it returns a missing lock owner error", id: uuid.New(), kind: repository.SharedLock, createdAt: now, expectedErr: repository.ErrMissingLockOwner},
		{name: "without creation date, then it returns a missing lock creation date error", id: uuid.New(), kind: repository.SharedLock, owner: lockOwner, expectedErr: repository.ErrMissingLockCreated},
		{name: "with valid data, then it returns the lock", id: uuid.New(), kind: repository.ExclusiveLock, owner: lockOwner, createdAt: now},
	}
	for _, tt := range tests {
		t.Run(`Given lock data,
		when HydrateLock is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			lock, err := repository.HydrateLock(tt.id, tt.kind, tt.owner, tt.createdAt)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.id, lock.ID())
			require.Equal(t, tt.kind, lock.Kind())
			require.Equal(t, tt.owner, lock.Owner())
			require.True(t, tt.createdAt.Equal(lock.CreatedAt()))
		})
	}
}

func TestLock_ConflictsWith(t *testing.T) {
	t.Parallel()

	now := time.Now()
	shared := newLock(t, repository.SharedLock, now)
	exclusive := newLock(t, repository.ExclusiveLock, now)

	tests := []struct {
		name     string
		lock     *repository.Lock
		other    *repository.Lock
		expected bool
	}{
		{name: "with itself, then it does not conflict", lock: exclusive, other: exclusive, expected: false},
		{name: "with two shared locks, then they do not conflict", lock: shared, other: newLock(t, repository.SharedLock, now), expected: false},
		{name: "with a shared lock and an exclusive lock, then they conflict", lock: shared, other: exclusive, expected: true},
		{name: "with an exclusive lock and a shared lock, then they conflict", lock: exclusive, other: shared, expected: true},
		{name: "with two exclusive locks, then they conflict", lock: exclusive, other: newLock(t, repository.ExclusiveLock, now), expected: true},
	}
	for _, tt := range tests {
		t.Run(`Given two locks,
		when ConflictsWith is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expected, tt.lock.ConflictsWith(tt.other))
		})
	}
}

func TestLock_IsStale(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name      string
		createdAt time.Time
		expected  bool
	}{
		{name: "refreshed recently, then it is not stale", createdAt: now.Add(-time.Minute), expected: false},
		{name: "refreshed in the future because of clock skew, then it is not stale", createdAt: now.Add(time.Hour), expected: false},
		{name: "not refreshed for longer than the stale period, then it is stale", createdAt: now.Add(-repository.LockStaleAfter - time.Second), expected: true},
	}
	for _, tt := range tests {
		t.Run(`Given a lock `+tt.name+`,
		when IsStale is called`, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expected, newLock(t, repository.SharedLock, tt.createdAt).IsStale(now))
		})
	}
}

func TestLock_Refresh(t *testing.T) {
	t.Parallel()

	t.Run(`Given a lock,
	when Refresh is called,
	then it returns a new lock with the same kind and owner and the new date`, func(t *testing.T) {
		t.Parallel()

		lock := newLock(t, repository.ExclusiveLock, time.Now().Add(-time.Minute))
		now := time.Now()

		refreshed := lock.Refresh(now)

		require.NotEqual(t, lock.ID(), refreshed.ID())
		require.Equal(t, lock.Kind(), refreshed.Kind())
		require.Equal(t, lock.Owner(), refreshed.Owner())
		require.True(t, now.Equal(refreshed.CreatedAt()))
		require.False(t, refreshed.ConflictsWith(refreshed))
	})
}

func TestLockedError(t *testing.T) {
	t.Parallel()

	t.Run(`Given a lock held by another process,
	when a LockedError is built,
	then it wraps the repository locked error and describes the holder`, func(t *testing.T) {
		t.Parallel()

		holder := newLock(t, repository.ExclusiveLock, time.Now())

		err := repository.NewLockedError(holder)

		require.ErrorIs(t, err, repository.ErrRepositoryLocked)
		require.Equal(t, holder, err.Holder())
		require.Contains(t, err.Error(), "exclusive lock held by host (pid 42)")
	})
}
