package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUnlock_Handle(t *testing.T) {
	t.Parallel()

	now := time.Now()
	staleAt := now.Add(-repository.LockStaleAfter - time.Minute)
	active := otherLock(t, repository.ExclusiveLock, now)
	stale := otherLock(t, repository.SharedLock, staleAt)
	otherStale := otherLock(t, repository.ExclusiveLock, staleAt)

	tests := []struct {
		name            string
		all             bool
		existing        []*repository.Lock
		expectedRemoved int
		expectedActive  int
		expectedLeft    []uuid.UUID
	}{
		{
			name: "without locks, then it removes nothing",
		},
		{
			name:            "with active and stale locks, then it removes only the stale ones",
			existing:        []*repository.Lock{active, stale, otherStale},
			expectedRemoved: 2,
			expectedActive:  1,
			expectedLeft:    []uuid.UUID{active.ID()},
		},
		{
			name:            "with all and active and stale locks, then it removes every lock",
			all:             true,
			existing:        []*repository.Lock{active, stale},
			expectedRemoved: 2,
		},
	}
	for _, tt := range tests {
		t.Run(`Given an Unlock command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			locks := newMemoryLocks(tt.existing...)

			events, err := app.NewUnlock(locks.mock).Handle(t.Context(), app.UnlockCommand{RepositoryPath: "repo", All: tt.all})

			require.NoError(t, err)
			require.Len(t, events, 1)
			got, ok := events[0].(*app.UnlockEvent)
			require.True(t, ok)
			require.Equal(t, app.UnlockCommandName, got.EventName())
			require.Equal(t, tt.expectedRemoved, got.RemovedLocks)
			require.Equal(t, tt.expectedActive, got.ActiveLocks)
			left := make([]uuid.UUID, 0)
			for _, lock := range locks.snapshot() {
				left = append(left, lock.ID())
			}
			require.ElementsMatch(t, tt.expectedLeft, left)
		})
	}
}

func TestUnlock_Handle_RepositoryErrors(t *testing.T) {
	t.Parallel()

	errRepo := errors.New("lock repository failed")

	t.Run(`Given a lock repository that cannot list locks,
	when the Unlock command handler is called,
	then it returns the error`, func(t *testing.T) {
		t.Parallel()

		locks := newMemoryLocks()
		locks.mock.ListFunc = func(context.Context) ([]repository.Lock, error) { return nil, errRepo }

		events, err := app.NewUnlock(locks.mock).Handle(t.Context(), app.UnlockCommand{All: true})

		require.ErrorIs(t, err, errRepo)
		require.Empty(t, events)
	})

	t.Run(`Given a lock repository that cannot delete locks,
	when the Unlock command handler is called with all,
	then it returns the error`, func(t *testing.T) {
		t.Parallel()

		locks := newMemoryLocks(otherLock(t, repository.SharedLock, time.Now()))
		locks.mock.DeleteFunc = func(context.Context, uuid.UUID) error { return errRepo }

		events, err := app.NewUnlock(locks.mock).Handle(t.Context(), app.UnlockCommand{All: true})

		require.ErrorIs(t, err, errRepo)
		require.Empty(t, events)
	})
}
