package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var otherOwner = repository.NewLockOwner("other-host", 7)

type memoryLocks struct {
	mu    sync.Mutex
	locks map[uuid.UUID]repository.Lock
	mock  *app.LockRepositoryMock
}

func newMemoryLocks(existing ...*repository.Lock) *memoryLocks {
	m := &memoryLocks{locks: map[uuid.UUID]repository.Lock{}}
	for _, lock := range existing {
		m.locks[lock.ID()] = *lock
	}
	m.mock = &app.LockRepositoryMock{
		SaveFunc: func(_ context.Context, lock *repository.Lock) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			m.locks[lock.ID()] = *lock
			return nil
		},
		ListFunc: func(context.Context) ([]repository.Lock, error) {
			return m.snapshot(), nil
		},
		DeleteFunc: func(_ context.Context, lockID uuid.UUID) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			delete(m.locks, lockID)
			return nil
		},
	}
	return m
}

func (m *memoryLocks) snapshot() []repository.Lock {
	m.mu.Lock()
	defer m.mu.Unlock()
	locks := make([]repository.Lock, 0, len(m.locks))
	for _, lock := range m.locks {
		locks = append(locks, lock)
	}
	return locks
}

func (m *memoryLocks) clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	clear(m.locks)
}

func newUnlockedLocker() *app.RepositoryLocker {
	return app.NewRepositoryLocker(newMemoryLocks().mock)
}

func otherLock(t *testing.T, kind repository.LockKind, createdAt time.Time) *repository.Lock {
	t.Helper()
	lock, err := repository.NewLock(kind, otherOwner, createdAt)
	require.NoError(t, err)
	return lock
}

func TestRepositoryLocker_WithLock(t *testing.T) {
	t.Parallel()

	errOperation := errors.New("operation failed")
	now := time.Now()

	tests := []struct {
		name              string
		kind              repository.LockKind
		existing          *repository.Lock
		operationErr      error
		expectedErr       error
		expectedOperation bool
		expectedRemaining int
	}{
		{
			name:              "without other locks, then it runs the operation and releases its lock",
			kind:              repository.ExclusiveLock,
			expectedOperation: true,
		},
		{
			name:              "for a shared lock while another shared lock is held, then it runs the operation",
			kind:              repository.SharedLock,
			existing:          otherLock(t, repository.SharedLock, now),
			expectedOperation: true,
			expectedRemaining: 1,
		},
		{
			name:              "for a shared lock while an exclusive lock is held, then it returns a repository locked error",
			kind:              repository.SharedLock,
			existing:          otherLock(t, repository.ExclusiveLock, now),
			expectedErr:       repository.ErrRepositoryLocked,
			expectedRemaining: 1,
		},
		{
			name:              "for an exclusive lock while a shared lock is held, then it returns a repository locked error",
			kind:              repository.ExclusiveLock,
			existing:          otherLock(t, repository.SharedLock, now),
			expectedErr:       repository.ErrRepositoryLocked,
			expectedRemaining: 1,
		},
		{
			name:              "for an exclusive lock while a stale exclusive lock is held, then it removes the stale lock and runs the operation",
			kind:              repository.ExclusiveLock,
			existing:          otherLock(t, repository.ExclusiveLock, now.Add(-repository.LockStaleAfter-time.Minute)),
			expectedOperation: true,
		},
		{
			name:              "and the operation fails, then it returns the operation error and releases its lock",
			kind:              repository.SharedLock,
			operationErr:      errOperation,
			expectedErr:       errOperation,
			expectedOperation: true,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a RepositoryLocker,
		when WithLock is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			var existing []*repository.Lock
			if tt.existing != nil {
				existing = append(existing, tt.existing)
			}
			locks := newMemoryLocks(existing...)
			locker := app.NewRepositoryLocker(locks.mock)

			operationCalled := false
			err := locker.WithLock(t.Context(), tt.kind, func(context.Context) error {
				operationCalled = true
				held := locks.snapshot()
				require.Len(t, held, tt.expectedRemaining+1)
				return tt.operationErr
			})

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.expectedOperation, operationCalled)
			require.Len(t, locks.snapshot(), tt.expectedRemaining)
			if tt.existing != nil && tt.expectedRemaining == 1 {
				require.Equal(t, tt.existing.ID(), locks.snapshot()[0].ID())
			}
		})
	}
}

func TestRepositoryLocker_WithLock_HolderDetails(t *testing.T) {
	t.Parallel()

	t.Run(`Given a repository with an exclusive lock held by another process,
	when WithLock is called,
	then the error identifies the holder of the lock`, func(t *testing.T) {
		t.Parallel()

		holder := otherLock(t, repository.ExclusiveLock, time.Now())
		locker := app.NewRepositoryLocker(newMemoryLocks(holder).mock)

		err := locker.WithLock(t.Context(), repository.SharedLock, func(context.Context) error { return nil })

		var lockedErr repository.LockedError
		require.ErrorAs(t, err, &lockedErr)
		require.Equal(t, holder.ID(), lockedErr.Holder().ID())
		require.Contains(t, err.Error(), "other-host (pid 7)")
	})
}

func TestRepositoryLocker_WithLock_RepositoryErrors(t *testing.T) {
	t.Parallel()

	errRepo := errors.New("lock repository failed")

	t.Run(`Given a lock repository that cannot save locks,
	when WithLock is called,
	then it returns the error without running the operation`, func(t *testing.T) {
		t.Parallel()

		locks := newMemoryLocks()
		locks.mock.SaveFunc = func(context.Context, *repository.Lock) error { return errRepo }

		err := app.NewRepositoryLocker(locks.mock).WithLock(t.Context(), repository.SharedLock, func(context.Context) error {
			t.Fatal("operation must not run")
			return nil
		})

		require.ErrorIs(t, err, errRepo)
	})

	t.Run(`Given a lock repository that cannot list locks,
	when WithLock is called,
	then it returns the error, does not run the operation and removes its own lock`, func(t *testing.T) {
		t.Parallel()

		locks := newMemoryLocks()
		locks.mock.ListFunc = func(context.Context) ([]repository.Lock, error) { return nil, errRepo }

		err := app.NewRepositoryLocker(locks.mock).WithLock(t.Context(), repository.SharedLock, func(context.Context) error {
			t.Fatal("operation must not run")
			return nil
		})

		require.ErrorIs(t, err, errRepo)
		require.Empty(t, locks.snapshot())
	})
}

func TestRepositoryLocker_WithLock_Refresh(t *testing.T) {
	t.Parallel()

	t.Run(`Given a RepositoryLocker with a short refresh interval,
	when the operation lasts several intervals,
	then it replaces its lock with a fresh one and releases the last one`, func(t *testing.T) {
		t.Parallel()

		locks := newMemoryLocks()
		locker := app.NewRepositoryLocker(locks.mock, app.WithLockRefreshInterval(5*time.Millisecond))

		var first uuid.UUID
		err := locker.WithLock(t.Context(), repository.ExclusiveLock, func(context.Context) error {
			first = locks.snapshot()[0].ID()
			require.Eventually(t, func() bool {
				held := locks.snapshot()
				return len(held) == 1 && held[0].ID() != first
			}, time.Second, time.Millisecond)
			return nil
		})

		require.NoError(t, err)
		require.Empty(t, locks.snapshot())
		require.GreaterOrEqual(t, len(locks.mock.SaveCalls()), 2)
	})

	t.Run(`Given a RepositoryLocker with a short refresh interval,
	when another process removes its lock during the operation,
	then it cancels the operation and returns a lock lost error`, func(t *testing.T) {
		t.Parallel()

		locks := newMemoryLocks()
		locker := app.NewRepositoryLocker(locks.mock, app.WithLockRefreshInterval(5*time.Millisecond))

		err := locker.WithLock(t.Context(), repository.ExclusiveLock, func(ctx context.Context) error {
			locks.clear()
			<-ctx.Done()
			return ctx.Err()
		})

		require.ErrorIs(t, err, app.ErrLockLost)
		require.Empty(t, locks.snapshot())
	})

	t.Run(`Given a RepositoryLocker,
	when the caller context is cancelled during the operation,
	then it still releases its lock`, func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		locks := newMemoryLocks()
		locker := app.NewRepositoryLocker(locks.mock)

		err := locker.WithLock(ctx, repository.SharedLock, func(ctx context.Context) error {
			cancel()
			return ctx.Err()
		})

		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, locks.snapshot())
	})
}

func TestRekeyKey_Handle_Locked(t *testing.T) {
	t.Parallel()

	t.Run(`Given a repository where a snapshot is being created,
	when the RekeyKey command handler is called,
	then it returns a repository locked error without touching the repository`, func(t *testing.T) {
		t.Parallel()

		folderRepo := &app.FolderRepositoryMock{}
		objectRepo := &app.ObjectRepositoryMock{}
		locker := app.NewRepositoryLocker(newMemoryLocks(otherLock(t, repository.SharedLock, time.Now())).mock)
		handler := app.NewRekeyKey(folderRepo, objectRepo, &app.AsymmetricKeyPairRepositoryMock{}, &app.SymmetricKeyRepositoryMock{}, locker)

		events, err := handler.Handle(t.Context(), app.RekeyKeyCommand{RepositoryPath: "repo", PrivateKeyPath: "private", PublicKeyPath: "public"})

		require.ErrorIs(t, err, repository.ErrRepositoryLocked)
		require.Empty(t, events)
		require.Empty(t, folderRepo.GetConfigCalls())
		require.Empty(t, objectRepo.ListEncryptedObjectsCalls())
	})
}

func TestCreateSnapshot_Handle_Locked(t *testing.T) {
	t.Parallel()

	t.Run(`Given a repository whose key is being rotated,
	when the CreateSnapshot command handler is called,
	then it returns a repository locked error without saving objects`, func(t *testing.T) {
		t.Parallel()

		deps := newCreateDependencies(t, sourceFiles(3))
		locker := app.NewRepositoryLocker(newMemoryLocks(otherLock(t, repository.ExclusiveLock, time.Now())).mock)
		handler := app.NewCreateSnapshot(deps.sourceRepo, deps.manifestRepo, deps.objectRepo, deps.folderRepo, nil, nil, locker)

		_, err := createSnapshot(t.Context(), handler, createCommand)

		require.ErrorIs(t, err, repository.ErrRepositoryLocked)
		require.Empty(t, deps.objectRepo.SaveChunkCalls())
		require.Empty(t, deps.manifestRepo.SaveCalls())
	})
}
