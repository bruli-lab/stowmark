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

	t.Run(`Given a ForgetSnapshots command handler with three snapshots,
	when Handle method is called with keep last one,
	then it deletes the older manifests and returns one snapshot forgotten event for each`, func(t *testing.T) {
		t.Parallel()

		manifests := manifestsCreated(time.Hour, 2*time.Hour, 3*time.Hour)
		repo, deleted := forgetManifestRepo(manifests, nil, nil)

		events, err := app.NewForgetSnapshots(repo, newUnlockedLocker()).Handle(t.Context(), app.ForgetSnapshotsCommand{
			RepositoryPath: "repo",
			Policy:         retentionPolicy(t, snapshot.RetentionRules{Last: 1}),
		})

		require.NoError(t, err)
		forgotten := app.EventsOf[*snapshot.SnapshotForgotten](events)
		require.Len(t, events, 2)
		require.Len(t, forgotten, 2)
		ids := make([]string, len(forgotten))
		for i, ev := range forgotten {
			require.Equal(t, snapshot.SnapshotForgottenEventName, ev.EventName())
			require.Equal(t, "source", ev.Source())
			ids[i] = ev.SnapshotID()
		}
		require.Equal(t, []string{manifests[1].ID(), manifests[2].ID()}, ids)
		require.Equal(t, ids, deleted())
	})

	t.Run(`Given a ForgetSnapshots command handler whose policy keeps every snapshot,
	when Handle method is called,
	then it deletes nothing and returns no events`, func(t *testing.T) {
		t.Parallel()

		repo, deleted := forgetManifestRepo(manifestsCreated(time.Hour, 2*time.Hour), nil, nil)

		events, err := app.NewForgetSnapshots(repo, newUnlockedLocker()).Handle(t.Context(), app.ForgetSnapshotsCommand{
			RepositoryPath: "repo",
			Policy:         retentionPolicy(t, snapshot.RetentionRules{Last: 5}),
		})

		require.NoError(t, err)
		require.Empty(t, events)
		require.Empty(t, deleted())
	})
}

func TestForgetPlan_Handle(t *testing.T) {
	t.Parallel()

	t.Run(`Given a ForgetPlan query handler with three snapshots,
	when Handle method is called with keep last one,
	then it returns every decision newest first without deleting any manifest`, func(t *testing.T) {
		t.Parallel()

		manifests := manifestsCreated(time.Hour, 2*time.Hour, 3*time.Hour)
		repo, deleted := forgetManifestRepo(manifests, nil, nil)

		result, err := app.NewForgetPlan(repo, newUnlockedLocker()).Handle(t.Context(), app.ForgetPlanQuery{
			Policy: retentionPolicy(t, snapshot.RetentionRules{Last: 1}),
		})

		require.NoError(t, err)
		decisions, ok := result.([]snapshot.RetentionDecision)
		require.True(t, ok)
		require.Len(t, decisions, 3)
		require.Equal(t, manifests[0].ID(), decisions[0].Snapshot().ID())
		require.True(t, decisions[0].Keep())
		require.False(t, decisions[1].Keep())
		require.False(t, decisions[2].Keep())
		require.Empty(t, deleted())
	})

	t.Run(`Given a ForgetPlan query handler,
	when Handle method is called without a retention policy or with another query,
	then it returns an error`, func(t *testing.T) {
		t.Parallel()

		repo, _ := forgetManifestRepo(nil, nil, nil)
		handler := app.NewForgetPlan(repo, newUnlockedLocker())

		_, err := handler.Handle(t.Context(), app.ForgetPlanQuery{})
		require.ErrorIs(t, err, app.ErrMissingRetentionPolicy)
		_, err = handler.Handle(t.Context(), app.SnapshotListQuery{})
		require.Error(t, err)
	})
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
	then the command is blocked and the plan query is not`, func(t *testing.T) {
		t.Parallel()

		repo, deleted := forgetManifestRepo(manifestsCreated(time.Hour, 2*time.Hour), nil, nil)
		locker := app.NewRepositoryLocker(newMemoryLocks(otherLock(t, repository.SharedLock, time.Now())).mock)
		handler := app.NewForgetSnapshots(repo, locker)
		policy := retentionPolicy(t, snapshot.RetentionRules{Last: 1})

		_, err := handler.Handle(t.Context(), app.ForgetSnapshotsCommand{RepositoryPath: "repo", Policy: policy})
		require.ErrorIs(t, err, repository.ErrRepositoryLocked)
		require.Empty(t, repo.ListCalls())

		_, err = app.NewForgetPlan(repo, locker).Handle(t.Context(), app.ForgetPlanQuery{Policy: policy})
		require.NoError(t, err)
		require.Empty(t, deleted())
	})
}
