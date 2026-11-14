package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

type pruneDependencies struct {
	config       repository.Config
	folderRepo   *app.FolderRepositoryMock
	manifestRepo *app.ManifestRepositoryMock
	objectRepo   *app.ObjectRepositoryMock
	mu           sync.Mutex
	deleted      []string
}

func (d *pruneDependencies) deletedObjects() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.deleted
}

type pruneSetup struct {
	config      *encryption.EncryptionConfig
	manifests   []snapshot.Manifest
	stored      []string
	listErr     error
	deleteErr   error
	manifestErr error
}

func newPruneDependencies(t *testing.T, setup *pruneSetup) *pruneDependencies {
	t.Helper()

	config := fixtures.ConfigBuilder{EncryptionConfig: setup.config}.Build(t)
	deps := &pruneDependencies{config: config}
	deps.folderRepo = &app.FolderRepositoryMock{
		GetConfigFunc: func(context.Context, string) (*repository.Config, error) { return &config, nil },
	}
	deps.manifestRepo = &app.ManifestRepositoryMock{
		ListFunc: func(context.Context) ([]snapshot.Manifest, error) { return setup.manifests, setup.manifestErr },
	}
	deps.objectRepo = &app.ObjectRepositoryMock{
		ListObjectsFunc: func(context.Context, bool, uint64) ([]string, error) { return setup.stored, setup.listErr },
		DeleteObjectFunc: func(_ context.Context, hash string, _ bool, _ uint64) error {
			deps.mu.Lock()
			defer deps.mu.Unlock()
			deps.deleted = append(deps.deleted, hash)
			return setup.deleteErr
		},
	}
	return deps
}

func manifestWithHashes(hashes ...string) snapshot.Manifest {
	files := make([]snapshot.File, len(hashes))
	for i := range hashes {
		files[i] = fixtures.FileBuilder{Hash: &hashes[i]}.Build()
	}
	return fixtures.ManifestBuilder{Files: files}.Build()
}

func TestPruneRepository_Handle(t *testing.T) {
	t.Parallel()

	manifests := []snapshot.Manifest{manifestWithHashes("aa01", "bb02"), manifestWithHashes("bb02", "cc03")}
	stored := []string{"aa01", "bb02", "dd04", "ee05"}

	tests := []struct {
		name               string
		config             *encryption.EncryptionConfig
		expectedEncrypted  bool
		expectedGeneration uint64
	}{
		{
			name: "on a plain repository, then it deletes the unreferenced plain objects",
		},
		{
			name:               "on an encrypted repository, then it deletes the unreferenced objects of the current generation",
			config:             encryption.NewEncryptionConfig("key", "fingerprint", 3),
			expectedEncrypted:  true,
			expectedGeneration: 3,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a PruneRepository command handler with shared, orphan and missing objects,
		when Handle method is called `+tt.name+` and returns the missing and pruned objects events`, func(t *testing.T) {
			t.Parallel()

			deps := newPruneDependencies(t, &pruneSetup{config: tt.config, manifests: manifests, stored: stored})
			handler := app.NewPruneRepositoryWithWorkers(deps.folderRepo, deps.manifestRepo, deps.objectRepo, newUnlockedLocker(), 2)

			events, err := handler.Handle(t.Context(), app.PruneRepositoryCommand{RepositoryPath: "repo"})

			require.NoError(t, err)
			require.Len(t, events, 2)
			missing, err := app.SingleEvent[*snapshot.MissingObjectsDetected](events)
			require.NoError(t, err)
			require.Equal(t, snapshot.MissingObjectsDetectedEventName, missing.EventName())
			require.Equal(t, []string{"cc03"}, missing.Hashes())
			pruned, err := app.SingleEvent[*snapshot.ObjectsPruned](events)
			require.NoError(t, err)
			require.Equal(t, snapshot.ObjectsPrunedEventName, pruned.EventName())
			require.Equal(t, 2, pruned.Removed())
			require.Equal(t, deps.config.ID().String(), pruned.RepositoryID())
			require.Len(t, deps.objectRepo.ListObjectsCalls(), 1)
			require.Equal(t, tt.expectedEncrypted, deps.objectRepo.ListObjectsCalls()[0].Encrypted)
			require.Equal(t, tt.expectedGeneration, deps.objectRepo.ListObjectsCalls()[0].Generation)
			require.ElementsMatch(t, []string{"dd04", "ee05"}, deps.deletedObjects())
			for _, call := range deps.objectRepo.DeleteObjectCalls() {
				require.Equal(t, tt.expectedEncrypted, call.Encrypted)
				require.Equal(t, tt.expectedGeneration, call.Generation)
			}
		})
	}

	t.Run(`Given a PruneRepository command handler where every stored object is referenced,
	when Handle method is called,
	then it deletes nothing and returns no events`, func(t *testing.T) {
		t.Parallel()

		deps := newPruneDependencies(t, &pruneSetup{manifests: manifests, stored: []string{"aa01", "bb02", "cc03"}})

		events, err := app.NewPruneRepository(deps.folderRepo, deps.manifestRepo, deps.objectRepo, newUnlockedLocker()).
			Handle(t.Context(), app.PruneRepositoryCommand{RepositoryPath: "repo"})

		require.NoError(t, err)
		require.Empty(t, events)
		require.Empty(t, deps.deletedObjects())
	})
}

func TestPrunePlan_Handle(t *testing.T) {
	t.Parallel()

	t.Run(`Given a PrunePlan query handler with shared, orphan and missing objects,
	when Handle method is called,
	then it returns the plan without deleting any object`, func(t *testing.T) {
		t.Parallel()

		deps := newPruneDependencies(t, &pruneSetup{
			manifests: []snapshot.Manifest{manifestWithHashes("aa01", "bb02"), manifestWithHashes("bb02", "cc03")},
			stored:    []string{"aa01", "bb02", "dd04", "ee05"},
		})

		result, err := app.NewPrunePlan(deps.folderRepo, deps.manifestRepo, deps.objectRepo, newUnlockedLocker()).
			Handle(t.Context(), app.PrunePlanQuery{RepositoryPath: "repo"})

		require.NoError(t, err)
		plan, ok := result.(*snapshot.PrunePlan)
		require.True(t, ok)
		require.Equal(t, 4, plan.StoredObjects())
		require.Equal(t, 3, plan.ReferencedObjects())
		require.Equal(t, []string{"dd04", "ee05"}, plan.Unreferenced())
		require.Equal(t, []string{"cc03"}, plan.Missing())
		require.Empty(t, deps.deletedObjects())
	})

	t.Run(`Given a PrunePlan query handler whose objects cannot be listed,
	when Handle method is called,
	then it returns the error`, func(t *testing.T) {
		t.Parallel()

		errRepo := errors.New("repository failed")
		deps := newPruneDependencies(t, &pruneSetup{listErr: errRepo})

		_, err := app.NewPrunePlan(deps.folderRepo, deps.manifestRepo, deps.objectRepo, newUnlockedLocker()).
			Handle(t.Context(), app.PrunePlanQuery{RepositoryPath: "repo"})

		require.ErrorIs(t, err, errRepo)
	})
}

func TestPruneRepository_Handle_Errors(t *testing.T) {
	t.Parallel()

	errRepo := errors.New("repository failed")
	manifests := []snapshot.Manifest{manifestWithHashes("aa01")}

	tests := []struct {
		name  string
		setup pruneSetup
	}{
		{
			name:  "and the manifests cannot be listed, then it returns the error without deleting objects",
			setup: pruneSetup{manifestErr: errRepo, stored: []string{"aa01", "bb02"}},
		},
		{
			name:  "and the objects cannot be listed, then it returns the error",
			setup: pruneSetup{manifests: manifests, listErr: errRepo},
		},
		{
			name:  "and an object cannot be deleted, then it returns the error",
			setup: pruneSetup{manifests: manifests, stored: []string{"aa01", "bb02"}, deleteErr: errRepo},
		},
	}
	for _, tt := range tests {
		t.Run(`Given a PruneRepository command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			deps := newPruneDependencies(t, &tt.setup)

			events, err := app.NewPruneRepository(deps.folderRepo, deps.manifestRepo, deps.objectRepo, newUnlockedLocker()).
				Handle(t.Context(), app.PruneRepositoryCommand{RepositoryPath: "repo"})

			require.ErrorIs(t, err, errRepo)
			require.Empty(t, events)
			if tt.setup.manifestErr != nil {
				require.Empty(t, deps.objectRepo.DeleteObjectCalls())
			}
		})
	}
}

func TestPruneRepository_Handle_Locked(t *testing.T) {
	t.Parallel()

	t.Run(`Given a repository where a snapshot is being created,
	when the PruneRepository command handler is called,
	then the command is blocked without listing objects and the plan query is not`, func(t *testing.T) {
		t.Parallel()

		deps := newPruneDependencies(t, &pruneSetup{manifests: []snapshot.Manifest{manifestWithHashes("aa01")}, stored: []string{"aa01", "bb02"}})
		locker := app.NewRepositoryLocker(newMemoryLocks(otherLock(t, repository.SharedLock, time.Now())).mock)
		handler := app.NewPruneRepository(deps.folderRepo, deps.manifestRepo, deps.objectRepo, locker)

		_, err := handler.Handle(t.Context(), app.PruneRepositoryCommand{RepositoryPath: "repo"})
		require.ErrorIs(t, err, repository.ErrRepositoryLocked)
		require.Empty(t, deps.objectRepo.ListObjectsCalls())

		_, err = app.NewPrunePlan(deps.folderRepo, deps.manifestRepo, deps.objectRepo, locker).
			Handle(t.Context(), app.PrunePlanQuery{RepositoryPath: "repo"})
		require.NoError(t, err)
		require.Empty(t, deps.deletedObjects())
	})
}
