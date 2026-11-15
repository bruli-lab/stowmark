package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

type extendSetup struct {
	lock       *repository.ObjectLock
	until      map[string]time.Duration
	listErr    error
	readErr    error
	retainErr  error
	missingIDs []string
}

type extendDependencies struct {
	config        repository.Config
	folderRepo    *app.FolderRepositoryMock
	manifestRepo  *app.ManifestRepositoryMock
	retentionRepo *app.RetentionRepositoryMock
	mu            sync.Mutex
	retained      []string
}

func (d *extendDependencies) retainedItems() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.retained
}

func newExtendDependencies(t *testing.T, setup *extendSetup) *extendDependencies {
	t.Helper()

	built := fixtures.ConfigBuilder{}.Build(t)
	deps := &extendDependencies{config: *built.WithObjectLock(setup.lock)}
	deps.folderRepo = &app.FolderRepositoryMock{
		GetConfigFunc: func(context.Context, string) (*repository.Config, error) { return &deps.config, nil },
	}
	id := "s1"
	manifest := manifestWithHashes("aa01", "bb02")
	deps.manifestRepo = &app.ManifestRepositoryMock{
		ListFunc: func(context.Context, repository.Codec) ([]snapshot.Manifest, error) {
			return []snapshot.Manifest{*snapshot.NewManifest(id, manifest.Files(), manifest.CreatedAt(), manifest.Source(), manifest.Compression())}, setup.listErr
		},
	}
	deps.retentionRepo = &app.RetentionRepositoryMock{
		RetainedUntilFunc: func(_ context.Context, item repository.RetainedItem) (time.Time, error) {
			for _, missing := range setup.missingIDs {
				if item.ID() == missing {
					return time.Time{}, repository.NewNotFoundError(missing)
				}
			}
			return time.Now().Add(setup.until[item.ID()]), setup.readErr
		},
		RetainFunc: func(_ context.Context, item repository.RetainedItem, _ repository.Retention) error {
			deps.mu.Lock()
			defer deps.mu.Unlock()
			deps.retained = append(deps.retained, item.ID())
			return setup.retainErr
		},
	}
	return deps
}

func (d *extendDependencies) handler() *app.ExtendRetention {
	return app.NewExtendRetention(d.folderRepo, d.manifestRepo, d.retentionRepo, nil, nil, newUnlockedLocker())
}

func TestExtendRetention_Handle(t *testing.T) {
	t.Parallel()

	lock, err := repository.NewObjectLock(repository.GovernanceObjectLock, 30*24*time.Hour)
	require.NoError(t, err)

	t.Run(`Given a repository with object lock where the config and one object expire soon,
	when the ExtendRetention command handler is called,
	then it extends only them and returns the retention extended event`, func(t *testing.T) {
		t.Parallel()

		deps := newExtendDependencies(t, &extendSetup{
			lock: lock,
			until: map[string]time.Duration{
				"":     time.Hour,
				"s1":   29 * 24 * time.Hour,
				"aa01": 2 * 24 * time.Hour,
				"bb02": 20 * 24 * time.Hour,
			},
		})

		before := time.Now()
		events, err := deps.handler().Handle(t.Context(), app.ExtendRetentionCommand{RepositoryPath: "repo"})

		require.NoError(t, err)
		extended, err := app.SingleEvent[*repository.RetentionExtended](events)
		require.NoError(t, err)
		require.Equal(t, repository.RetentionExtendedEventName, extended.EventName())
		require.Equal(t, deps.config.ID().String(), extended.RepositoryID())
		require.Equal(t, 4, extended.Checked())
		require.Equal(t, 2, extended.Extended())
		require.WithinDuration(t, before.Add(lock.Period()), extended.Until(), 2*time.Second)
		require.ElementsMatch(t, []string{"", "aa01"}, deps.retainedItems())
		for _, call := range deps.retentionRepo.RetainCalls() {
			require.Equal(t, repository.GovernanceObjectLock, call.Retention.Mode())
			require.Equal(t, extended.Until(), call.Retention.Until())
		}
	})

	t.Run(`Given a repository with object lock where every item is protected long enough and one object is missing,
	when the ExtendRetention command handler is called,
	then it extends nothing and returns no events`, func(t *testing.T) {
		t.Parallel()

		deps := newExtendDependencies(t, &extendSetup{
			lock:       lock,
			until:      map[string]time.Duration{"": 29 * 24 * time.Hour, "s1": 29 * 24 * time.Hour, "aa01": 29 * 24 * time.Hour},
			missingIDs: []string{"bb02"},
		})

		events, err := deps.handler().Handle(t.Context(), app.ExtendRetentionCommand{RepositoryPath: "repo"})

		require.NoError(t, err)
		require.Empty(t, events)
		require.Empty(t, deps.retainedItems())
	})
}

func TestExtendRetention_Handle_Errors(t *testing.T) {
	t.Parallel()

	errTest := errors.New("test error")
	lock, err := repository.NewObjectLock(repository.GovernanceObjectLock, time.Hour)
	require.NoError(t, err)

	tests := []struct {
		name        string
		setup       extendSetup
		expectedErr error
	}{
		{
			name:        "on a repository without object lock, then it returns object lock disabled error",
			expectedErr: repository.ErrObjectLockDisabled,
		},
		{
			name:        "and list manifests fails, then it returns the same error",
			setup:       extendSetup{lock: lock, listErr: errTest},
			expectedErr: errTest,
		},
		{
			name:        "and reading a retention fails, then it returns the same error",
			setup:       extendSetup{lock: lock, readErr: errTest},
			expectedErr: errTest,
		},
		{
			name:        "and extending a retention fails, then it returns the same error",
			setup:       extendSetup{lock: lock, retainErr: errTest},
			expectedErr: errTest,
		},
	}
	for _, tt := range tests {
		t.Run(`Given an ExtendRetention command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			deps := newExtendDependencies(t, &tt.setup)

			events, err := deps.handler().Handle(t.Context(), app.ExtendRetentionCommand{RepositoryPath: "repo"})

			require.ErrorIs(t, err, tt.expectedErr)
			require.Empty(t, events)
		})
	}
}
