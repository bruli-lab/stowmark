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

func manifestsCreated(ages ...time.Duration) []snapshot.Manifest {
	now := time.Now()
	manifests := make([]snapshot.Manifest, len(ages))
	for i, age := range ages {
		id := snapshot.NewID() + string(rune('a'+i))
		createdAt := now.Add(-age)
		manifests[i] = fixtures.ManifestBuilder{ID: &id, CreatedAt: &createdAt}.Build()
	}
	return manifests
}

func retentionPolicy(t *testing.T, rules snapshot.RetentionRules) *snapshot.RetentionPolicy {
	t.Helper()
	policy, err := snapshot.NewRetentionPolicy(rules)
	require.NoError(t, err)
	return policy
}

func forgetManifestRepo(manifests []snapshot.Manifest, listErr, deleteErr error) (repo *app.ManifestRepositoryMock, deleted func() []string) {
	var (
		mu  sync.Mutex
		ids []string
	)
	repo = &app.ManifestRepositoryMock{
		ListFunc: func(context.Context) ([]snapshot.Manifest, error) { return manifests, listErr },
		DeleteFunc: func(_ context.Context, snapshotID string) error {
			mu.Lock()
			defer mu.Unlock()
			ids = append(ids, snapshotID)
			return deleteErr
		},
	}
	deleted = func() []string {
		mu.Lock()
		defer mu.Unlock()
		return ids
	}
	return repo, deleted
}

func TestForgetSnapshots_Handle(t *testing.T) {
	t.Parallel()

	manifests := manifestsCreated(time.Hour, 2*time.Hour, 3*time.Hour)

	tests := []struct {
		name              string
		dryRun            bool
		expectedDeleted   []string
		expectedForgotten int
	}{
		{
			name:              "with keep last one, then it deletes the older manifests",
			expectedDeleted:   []string{manifests[1].ID(), manifests[2].ID()},
			expectedForgotten: 2,
		},
		{
			name:              "with keep last one in dry run, then it reports the older manifests without deleting them",
			dryRun:            true,
			expectedForgotten: 2,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a ForgetSnapshots command handler with three snapshots,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			repo, deleted := forgetManifestRepo(manifests, nil, nil)
			handler := app.NewForgetSnapshots(repo, newUnlockedLocker())

			events, err := handler.Handle(t.Context(), app.ForgetSnapshotsCommand{
				RepositoryPath: "repo",
				Policy:         retentionPolicy(t, snapshot.RetentionRules{Last: 1}),
				DryRun:         tt.dryRun,
			})

			require.NoError(t, err)
			require.Len(t, events, 1)
			got, ok := events[0].(*app.ForgetSnapshotsEvent)
			require.True(t, ok)
			require.Equal(t, app.ForgetSnapshotsCommandName, got.EventName())
			require.Equal(t, tt.dryRun, got.DryRun)
			require.Len(t, got.Decisions, 3)
			require.Equal(t, manifests[0].ID(), got.Decisions[0].Snapshot().ID())
			require.True(t, got.Decisions[0].Keep())
			require.Equal(t, tt.expectedForgotten, got.Forgotten())
			require.ElementsMatch(t, tt.expectedDeleted, deleted())
		})
	}
}

func TestForgetSnapshots_Handle_Errors(t *testing.T) {
	t.Parallel()

	errRepo := errors.New("manifest repository failed")
	manifests := manifestsCreated(time.Hour, 2*time.Hour)

	tests := []struct {
		name        string
		policy      bool
		listErr     error
		deleteErr   error
		expectedErr error
	}{
		{
			name:        "without a retention policy, then it returns a missing retention policy error",
			expectedErr: app.ErrMissingRetentionPolicy,
		},
		{
			name:        "and the manifests cannot be listed, then it returns the error",
			policy:      true,
			listErr:     errRepo,
			expectedErr: errRepo,
		},
		{
			name:        "and a manifest cannot be deleted, then it returns the error",
			policy:      true,
			deleteErr:   errRepo,
			expectedErr: errRepo,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a ForgetSnapshots command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			repo, _ := forgetManifestRepo(manifests, tt.listErr, tt.deleteErr)
			command := app.ForgetSnapshotsCommand{RepositoryPath: "repo"}
			if tt.policy {
				command.Policy = retentionPolicy(t, snapshot.RetentionRules{Last: 1})
			}

			events, err := app.NewForgetSnapshots(repo, newUnlockedLocker()).Handle(t.Context(), command)

			require.ErrorIs(t, err, tt.expectedErr)
			require.Empty(t, events)
		})
	}
}

func TestForgetSnapshots_Handle_Locked(t *testing.T) {
	t.Parallel()

	t.Run(`Given a repository where a snapshot is being created,
	when the ForgetSnapshots command handler is called,
	then it is blocked unless it is a dry run`, func(t *testing.T) {
		t.Parallel()

		repo, deleted := forgetManifestRepo(manifestsCreated(time.Hour, 2*time.Hour), nil, nil)
		locker := app.NewRepositoryLocker(newMemoryLocks(otherLock(t, repository.SharedLock, time.Now())).mock)
		handler := app.NewForgetSnapshots(repo, locker)
		command := app.ForgetSnapshotsCommand{RepositoryPath: "repo", Policy: retentionPolicy(t, snapshot.RetentionRules{Last: 1})}

		_, err := handler.Handle(t.Context(), command)
		require.ErrorIs(t, err, repository.ErrRepositoryLocked)
		require.Empty(t, repo.ListCalls())

		command.DryRun = true
		_, err = handler.Handle(t.Context(), command)
		require.NoError(t, err)
		require.Empty(t, deleted())
	})
}
