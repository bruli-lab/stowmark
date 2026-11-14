package repository_test

import (
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestRepositoryEvents(t *testing.T) {
	t.Parallel()

	t.Run(`Given a written repository config,
	when Initialized is called with warnings,
	then the event belongs to the repository and carries the warnings`, func(t *testing.T) {
		t.Parallel()

		conf := fixtures.ConfigBuilder{}.Build(t)

		initialized := conf.Initialized([]string{repository.EncryptionPreservedWarning})

		require.Equal(t, repository.RepositoryInitializedEventName, initialized.EventName())
		require.Equal(t, conf.ID().String(), initialized.RepositoryID())
		require.Equal(t, []string{repository.EncryptionPreservedWarning}, initialized.Warnings())
	})

	t.Run(`Given a deleted lock,
	when Removed is called,
	then the event belongs to the lock and keeps who held it`, func(t *testing.T) {
		t.Parallel()

		lock, err := repository.NewLock(repository.ExclusiveLock, repository.NewLockOwner("host", 7), time.Now())
		require.NoError(t, err)

		removed := lock.Removed()

		require.Equal(t, repository.LockRemovedEventName, removed.EventName())
		require.Equal(t, lock.ID().String(), removed.AggregateRootID())
		require.Equal(t, lock, removed.Lock())
	})
}
