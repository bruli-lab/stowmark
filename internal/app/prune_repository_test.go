package app_test

import (
	"context"
	"errors"
	"fmt"
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
	config        repository.Config
	folderRepo    *app.FolderRepositoryMock
	manifestRepo  *app.ManifestRepositoryMock
	objectRepo    *app.ObjectRepositoryMock
	encryptedRepo *app.EncryptedObjectRepositoryMock
	mu            sync.Mutex
	deleted       []string
}

func (d *pruneDependencies) deletedObjects() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.deleted
}

const storedObjectSize = 10

type pruneSetup struct {
	config         *encryption.EncryptionConfig
	manifests      []snapshot.Manifest
	stored         []string
	generations    []uint64
	temporaries    int
	listErr        error
	deleteErr      error
	manifestErr    error
	generationsErr error
	deleteGenErr   error
	temporariesErr error
	retained       map[string]time.Time
	retainedGens   map[uint64]time.Time
}

func newPruneDependencies(t *testing.T, setup *pruneSetup) *pruneDependencies {
	t.Helper()

	config := fixtures.ConfigBuilder{EncryptionConfig: setup.config}.Build(t)
	deps := &pruneDependencies{config: config}
	deps.folderRepo = &app.FolderRepositoryMock{
		GetConfigFunc: func(context.Context, string) (*repository.Config, error) { return &config, nil },
	}
	deps.manifestRepo = &app.ManifestRepositoryMock{
		ListFunc: func(context.Context, repository.Codec) ([]snapshot.Manifest, error) {
			return setup.manifests, setup.manifestErr
		},
	}
	deps.objectRepo = &app.ObjectRepositoryMock{
		ListObjectsFunc: func(context.Context, bool, uint64) ([]snapshot.StoredObject, error) {
			stored := make([]snapshot.StoredObject, len(setup.stored))
			for i, hash := range setup.stored {
				stored[i] = snapshot.NewStoredObject(hash, storedObjectSize)
			}
			return stored, setup.listErr
		},
		DeleteObjectFunc: func(_ context.Context, hash string, _ bool, _ uint64) error {
			deps.mu.Lock()
			defer deps.mu.Unlock()
			deps.deleted = append(deps.deleted, hash)
			if until, ok := setup.retained[hash]; ok {
				return fmt.Errorf("delete object %q: %w", hash, repository.NewRetainedError(until))
			}
			return setup.deleteErr
		},
		DeleteTemporaryObjectsFunc: func(context.Context) (int, error) { return setup.temporaries, setup.temporariesErr },
	}
	deps.encryptedRepo = &app.EncryptedObjectRepositoryMock{
		ListEncryptedGenerationsFunc: func(context.Context) ([]uint64, error) { return setup.generations, setup.generationsErr },
		DeleteEncryptedGenerationFunc: func(_ context.Context, generation uint64) error {
			if until, ok := setup.retainedGens[generation]; ok {
				return repository.NewRetainedError(until)
			}
			return setup.deleteGenErr
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
			handler := app.NewPruneRepositoryWithWorkers(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.encryptedRepo, nil, nil, newUnlockedLocker(), 2)

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
			require.Equal(t, int64(2*storedObjectSize), pruned.FreedBytes())
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

		events, err := app.NewPruneRepository(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.encryptedRepo, nil, nil, newUnlockedLocker()).
			Handle(t.Context(), app.PruneRepositoryCommand{RepositoryPath: "repo"})

		require.NoError(t, err)
		require.Empty(t, events)
		require.Empty(t, deps.deletedObjects())
	})
}

func TestPruneRepository_Handle_Retained(t *testing.T) {
	t.Parallel()

	t.Run(`Given an encrypted repository with object lock where an orphan object and an old generation are still retained,
	when the PruneRepository command handler is called,
	then it deletes what it can, postpones the rest and reports both`, func(t *testing.T) {
		t.Parallel()

		soon := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
		later := soon.Add(time.Hour)
		deps := newPruneDependencies(t, &pruneSetup{
			config:       encryption.NewEncryptionConfig("key", "fingerprint", 3),
			manifests:    []snapshot.Manifest{manifestWithHashes("aa01")},
			stored:       []string{"aa01", "bb02", "cc03"},
			generations:  []uint64{1, 2, 3},
			retained:     map[string]time.Time{"bb02": soon},
			retainedGens: map[uint64]time.Time{1: later},
		})

		events, err := app.NewPruneRepository(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.encryptedRepo, nil, nil, newUnlockedLocker()).
			Handle(t.Context(), app.PruneRepositoryCommand{RepositoryPath: "repo"})

		require.NoError(t, err)
		require.Len(t, events, 3)
		pruned, err := app.SingleEvent[*snapshot.ObjectsPruned](events)
		require.NoError(t, err)
		require.Equal(t, 1, pruned.Removed())
		require.Equal(t, int64(storedObjectSize), pruned.FreedBytes())
		removed, err := app.SingleEvent[*snapshot.GenerationsRemoved](events)
		require.NoError(t, err)
		require.Equal(t, []uint64{2}, removed.Generations())
		postponed, err := app.SingleEvent[*snapshot.ObjectsDeletionPostponed](events)
		require.NoError(t, err)
		require.Equal(t, snapshot.ObjectsDeletionPostponedEventName, postponed.EventName())
		require.Equal(t, 1, postponed.Objects())
		require.Equal(t, int64(storedObjectSize), postponed.Bytes())
		require.Equal(t, []uint64{1}, postponed.Generations())
		require.Equal(t, later, postponed.Until())
		require.Equal(t, deps.config.ID().String(), postponed.RepositoryID())
	})

	t.Run(`Given a repository where every orphan object is still retained,
	when the PruneRepository command handler is called,
	then it only returns the postponed event`, func(t *testing.T) {
		t.Parallel()

		until := time.Now().Add(time.Hour)
		deps := newPruneDependencies(t, &pruneSetup{
			manifests: []snapshot.Manifest{manifestWithHashes("aa01")},
			stored:    []string{"aa01", "bb02"},
			retained:  map[string]time.Time{"bb02": until},
		})

		events, err := app.NewPruneRepository(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.encryptedRepo, nil, nil, newUnlockedLocker()).
			Handle(t.Context(), app.PruneRepositoryCommand{RepositoryPath: "repo"})

		require.NoError(t, err)
		require.Len(t, events, 1)
		postponed, err := app.SingleEvent[*snapshot.ObjectsDeletionPostponed](events)
		require.NoError(t, err)
		require.Equal(t, 1, postponed.Objects())
		require.Empty(t, postponed.Generations())
	})
}

func TestPruneRepository_Handle_Leftovers(t *testing.T) {
	t.Parallel()

	t.Run(`Given an encrypted repository with objects of old key generations and temporary files,
	when the PruneRepository command handler is called,
	then it deletes every generation but the current one and the temporary files, and returns their events`, func(t *testing.T) {
		t.Parallel()

		deps := newPruneDependencies(t, &pruneSetup{
			config:      encryption.NewEncryptionConfig("key", "fingerprint", 3),
			manifests:   []snapshot.Manifest{manifestWithHashes("aa01")},
			stored:      []string{"aa01"},
			generations: []uint64{1, 2, 3},
			temporaries: 4,
		})

		events, err := app.NewPruneRepository(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.encryptedRepo, nil, nil, newUnlockedLocker()).
			Handle(t.Context(), app.PruneRepositoryCommand{RepositoryPath: "repo"})

		require.NoError(t, err)
		require.Len(t, events, 2)
		removed, err := app.SingleEvent[*snapshot.GenerationsRemoved](events)
		require.NoError(t, err)
		require.Equal(t, snapshot.GenerationsRemovedEventName, removed.EventName())
		require.Equal(t, []uint64{1, 2}, removed.Generations())
		require.Equal(t, deps.config.ID().String(), removed.RepositoryID())
		temporaries, err := app.SingleEvent[*snapshot.TemporariesRemoved](events)
		require.NoError(t, err)
		require.Equal(t, 4, temporaries.Removed())
		deletedGenerations := make([]uint64, 0)
		for _, call := range deps.encryptedRepo.DeleteEncryptedGenerationCalls() {
			deletedGenerations = append(deletedGenerations, call.Generation)
		}
		require.Equal(t, []uint64{1, 2}, deletedGenerations)
	})

	t.Run(`Given a plain repository with encrypted objects left from a previous initialisation,
	when the PruneRepository command handler is called,
	then it deletes every encrypted generation`, func(t *testing.T) {
		t.Parallel()

		deps := newPruneDependencies(t, &pruneSetup{generations: []uint64{1}})

		events, err := app.NewPruneRepository(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.encryptedRepo, nil, nil, newUnlockedLocker()).
			Handle(t.Context(), app.PruneRepositoryCommand{RepositoryPath: "repo"})

		require.NoError(t, err)
		removed, err := app.SingleEvent[*snapshot.GenerationsRemoved](events)
		require.NoError(t, err)
		require.Equal(t, []uint64{1}, removed.Generations())
	})

	t.Run(`Given an encrypted repository with old key generations,
	when the PrunePlan query handler is called,
	then the plan lists them without deleting anything`, func(t *testing.T) {
		t.Parallel()

		deps := newPruneDependencies(t, &pruneSetup{config: encryption.NewEncryptionConfig("key", "fingerprint", 2), generations: []uint64{1, 2}})

		result, err := app.NewPrunePlan(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.encryptedRepo, nil, nil, newUnlockedLocker()).
			Handle(t.Context(), app.PrunePlanQuery{RepositoryPath: "repo"})

		require.NoError(t, err)
		plan, ok := result.(*snapshot.PrunePlan)
		require.True(t, ok)
		require.Equal(t, []uint64{1}, plan.OrphanGenerations())
		require.Empty(t, deps.encryptedRepo.DeleteEncryptedGenerationCalls())
		require.Empty(t, deps.objectRepo.DeleteTemporaryObjectsCalls())
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

		result, err := app.NewPrunePlan(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.encryptedRepo, nil, nil, newUnlockedLocker()).
			Handle(t.Context(), app.PrunePlanQuery{RepositoryPath: "repo"})

		require.NoError(t, err)
		plan, ok := result.(*snapshot.PrunePlan)
		require.True(t, ok)
		require.Equal(t, 4, plan.StoredObjects())
		require.Equal(t, 3, plan.ReferencedObjects())
		require.Equal(t, []string{"dd04", "ee05"}, plan.Unreferenced())
		require.Equal(t, int64(2*storedObjectSize), plan.UnreferencedBytes())
		require.Equal(t, []string{"cc03"}, plan.Missing())
		require.Empty(t, deps.deletedObjects())
	})

	t.Run(`Given a PrunePlan query handler whose objects cannot be listed,
	when Handle method is called,
	then it returns the error`, func(t *testing.T) {
		t.Parallel()

		errRepo := errors.New("repository failed")
		deps := newPruneDependencies(t, &pruneSetup{listErr: errRepo})

		_, err := app.NewPrunePlan(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.encryptedRepo, nil, nil, newUnlockedLocker()).
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
		{
			name:  "and the key generations cannot be listed, then it returns the error without deleting objects",
			setup: pruneSetup{manifests: manifests, stored: []string{"aa01", "bb02"}, generationsErr: errRepo},
		},
		{
			name:  "and an old key generation cannot be deleted, then it returns the error",
			setup: pruneSetup{manifests: manifests, stored: []string{"aa01"}, generations: []uint64{1}, deleteGenErr: errRepo},
		},
		{
			name:  "and the temporary files cannot be deleted, then it returns the error",
			setup: pruneSetup{manifests: manifests, stored: []string{"aa01"}, temporariesErr: errRepo},
		},
	}
	for _, tt := range tests {
		t.Run(`Given a PruneRepository command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			deps := newPruneDependencies(t, &tt.setup)

			events, err := app.NewPruneRepository(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.encryptedRepo, nil, nil, newUnlockedLocker()).
				Handle(t.Context(), app.PruneRepositoryCommand{RepositoryPath: "repo"})

			require.ErrorIs(t, err, errRepo)
			require.Empty(t, events)
			if tt.setup.manifestErr != nil || tt.setup.generationsErr != nil {
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
		handler := app.NewPruneRepository(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.encryptedRepo, nil, nil, locker)

		_, err := handler.Handle(t.Context(), app.PruneRepositoryCommand{RepositoryPath: "repo"})
		require.ErrorIs(t, err, repository.ErrRepositoryLocked)
		require.Empty(t, deps.objectRepo.ListObjectsCalls())

		_, err = app.NewPrunePlan(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.encryptedRepo, nil, nil, locker).
			Handle(t.Context(), app.PrunePlanQuery{RepositoryPath: "repo"})
		require.NoError(t, err)
		require.Empty(t, deps.deletedObjects())
	})
}
