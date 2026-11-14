package app_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"strings"
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

var (
	healthyContent = "healthy object"
	corruptContent = "object whose bytes changed"
	missingContent = "object that was never stored"
	orphanContent  = "object no snapshot uses"
)

type checkSetup struct {
	encryption  *encryption.EncryptionConfig
	listIDsErr  error
	listObjErr  error
	generations []uint64
}

type checkDependencies struct {
	folderRepo   *app.FolderRepositoryMock
	manifestRepo *app.ManifestRepositoryMock
	objectRepo   *app.ObjectRepositoryMock
	lockRepo     *app.LockRepositoryMock
	mu           sync.Mutex
	readKeys     [][]byte
}

func newCheckDependencies(t *testing.T, setup checkSetup) *checkDependencies {
	t.Helper()

	config := fixtures.ConfigBuilder{EncryptionConfig: setup.encryption}.Build(t)
	manifests := map[string]snapshot.Manifest{
		"s1": manifestWithHashes(hashOf(healthyContent), hashOf(corruptContent)),
		"s2": manifestWithHashes(hashOf(healthyContent), hashOf(missingContent)),
	}
	deps := &checkDependencies{}
	deps.folderRepo = &app.FolderRepositoryMock{
		GetConfigFunc: func(context.Context, string) (*repository.Config, error) { return &config, nil },
	}
	deps.manifestRepo = &app.ManifestRepositoryMock{
		ListIDsFunc: func(context.Context) ([]string, error) { return []string{"broken", "s1", "s2"}, setup.listIDsErr },
		GetFunc: func(_ context.Context, id string) (*snapshot.Manifest, error) {
			manifest, ok := manifests[id]
			if !ok {
				return nil, errors.New("invalid manifest json")
			}
			hydrated := snapshot.NewManifest(id, manifest.Files(), manifest.CreatedAt(), manifest.Source(), manifest.Compression())
			return hydrated, nil
		},
	}
	deps.objectRepo = &app.ObjectRepositoryMock{
		ListObjectsFunc: func(context.Context, bool, uint64) ([]string, error) {
			return []string{hashOf(healthyContent), hashOf(corruptContent), hashOf(orphanContent)}, setup.listObjErr
		},
		ListEncryptedGenerationsFunc: func(context.Context) ([]uint64, error) { return setup.generations, nil },
		ReadObjectFunc: func(_ context.Context, hash string, key []byte, _ uint64) (io.ReadCloser, error) {
			deps.mu.Lock()
			deps.readKeys = append(deps.readKeys, key)
			deps.mu.Unlock()
			content := healthyContent
			if hash == hashOf(corruptContent) {
				content = "tampered"
			}
			return io.NopCloser(strings.NewReader(content)), nil
		},
	}
	stale, err := repository.NewLock(repository.ExclusiveLock, otherOwner, time.Now().Add(-repository.LockStaleAfter-time.Minute))
	require.NoError(t, err)
	deps.lockRepo = newMemoryLocks(stale).mock
	return deps
}

func (d *checkDependencies) handler(asymmetric *app.AsymmetricKeyPairRepositoryMock, symmetric *app.SymmetricKeyRepositoryMock) *app.CheckRepository {
	return app.NewCheckRepositoryWithWorkers(d.folderRepo, d.manifestRepo, d.objectRepo, d.lockRepo, asymmetric, symmetric, newUnlockedLocker(), 2)
}

func checkReport(t *testing.T, handler *app.CheckRepository, query app.CheckRepositoryQuery) *snapshot.CheckReport {
	t.Helper()
	result, err := handler.Handle(t.Context(), query)
	require.NoError(t, err)
	report, ok := result.(*snapshot.CheckReport)
	require.True(t, ok)
	return report
}

func keyRepositories() (*app.AsymmetricKeyPairRepositoryMock, *app.SymmetricKeyRepositoryMock) {
	asymmetric := &app.AsymmetricKeyPairRepositoryMock{
		ReadRSAPrivateKeyFunc: func(context.Context, string) (*rsa.PrivateKey, error) { return &rsa.PrivateKey{}, nil },
	}
	symmetric := &app.SymmetricKeyRepositoryMock{
		DecodeAndDecryptSymmetricKeyFunc: func(context.Context, *rsa.PrivateKey, string) ([]byte, error) { return []byte("repository-key"), nil },
	}
	return asymmetric, symmetric
}

func TestCheckRepository_Handle(t *testing.T) {
	t.Parallel()

	t.Run(`Given a plain repository with a corrupt manifest, a missing object, an orphan, a stale lock and encrypted leftovers,
	when the CheckRepository query handler is called without reading data,
	then it reports every structural problem without reading any object`, func(t *testing.T) {
		t.Parallel()

		deps := newCheckDependencies(t, checkSetup{generations: []uint64{1}})

		report := checkReport(t, deps.handler(nil, nil), app.CheckRepositoryQuery{RepositoryPath: "repo"})

		require.Equal(t, 2, report.Snapshots())
		require.Len(t, report.CorruptManifests(), 1)
		require.Equal(t, "broken", report.CorruptManifests()[0].SnapshotID())
		require.Equal(t, 3, report.StoredObjects())
		require.Equal(t, 3, report.ReferencedObjects())
		require.Equal(t, 1, report.UnreferencedObjects())
		require.Len(t, report.MissingObjects(), 1)
		require.Equal(t, hashOf(missingContent), report.MissingObjects()[0].Hash())
		require.Equal(t, []string{"s2"}, report.MissingObjects()[0].Snapshots())
		require.Equal(t, 1, report.StaleLocks())
		require.Equal(t, []uint64{1}, report.OrphanGenerations())
		require.False(t, report.ReadData())
		require.Empty(t, deps.objectRepo.ReadObjectCalls())
		require.True(t, report.HasErrors())
	})

	t.Run(`Given a plain repository with a tampered object,
	when the CheckRepository query handler is called reading data,
	then it reads each referenced object once and reports the tampered one`, func(t *testing.T) {
		t.Parallel()

		deps := newCheckDependencies(t, checkSetup{})

		report := checkReport(t, deps.handler(nil, nil), app.CheckRepositoryQuery{RepositoryPath: "repo", ReadData: true})

		require.True(t, report.ReadData())
		require.Equal(t, 2, report.ReadObjects())
		require.Len(t, deps.objectRepo.ReadObjectCalls(), 2)
		require.Len(t, report.CorruptObjects(), 1)
		require.Equal(t, hashOf(corruptContent), report.CorruptObjects()[0].Hash())
		require.Contains(t, report.CorruptObjects()[0].Reason(), "hash mismatch")
		for _, key := range deps.readKeys {
			require.Empty(t, key)
		}
	})

	t.Run(`Given an encrypted repository with an older generation,
	when the CheckRepository query handler is called reading data with the private key,
	then it reads the current generation with the repository key and only reports the older generation`, func(t *testing.T) {
		t.Parallel()

		deps := newCheckDependencies(t, checkSetup{encryption: encryption.NewEncryptionConfig("key", "fingerprint", 2), generations: []uint64{1, 2}})
		asymmetric, symmetric := keyRepositories()

		report := checkReport(t, deps.handler(asymmetric, symmetric), app.CheckRepositoryQuery{RepositoryPath: "repo", ReadData: true, PrivateKeyPath: new("private.pem")})

		require.Equal(t, []uint64{1}, report.OrphanGenerations())
		require.True(t, deps.objectRepo.ListObjectsCalls()[0].Encrypted)
		require.Equal(t, uint64(2), deps.objectRepo.ListObjectsCalls()[0].Generation)
		for _, call := range deps.objectRepo.ReadObjectCalls() {
			require.Equal(t, []byte("repository-key"), call.SymmetricKey)
			require.Equal(t, uint64(2), call.Generation)
		}
	})

	t.Run(`Given an encrypted repository with an object that cannot be decrypted,
	when the CheckRepository query handler is called reading data,
	then it reports the object as corrupt instead of failing`, func(t *testing.T) {
		t.Parallel()

		deps := newCheckDependencies(t, checkSetup{encryption: encryption.NewEncryptionConfig("key", "fingerprint", 2)})
		deps.objectRepo.ReadObjectFunc = func(_ context.Context, hash string, _ []byte, _ uint64) (io.ReadCloser, error) {
			if hash == hashOf(corruptContent) {
				return nil, fmt.Errorf("%w: decrypt object %q", snapshot.ErrCorruptObject, hash)
			}
			return io.NopCloser(strings.NewReader(healthyContent)), nil
		}
		asymmetric, symmetric := keyRepositories()

		report := checkReport(t, deps.handler(asymmetric, symmetric), app.CheckRepositoryQuery{RepositoryPath: "repo", ReadData: true, PrivateKeyPath: new("private.pem")})

		require.Len(t, report.CorruptObjects(), 1)
		require.Equal(t, hashOf(corruptContent), report.CorruptObjects()[0].Hash())
		require.Contains(t, report.CorruptObjects()[0].Reason(), "corrupt object")
	})

	t.Run(`Given a plain repository,
	when the CheckRepository query handler is called with a data subset,
	then it reads only the selected objects`, func(t *testing.T) {
		t.Parallel()

		deps := newCheckDependencies(t, checkSetup{})
		subset, err := snapshot.ParseDataSubset("50%")
		require.NoError(t, err)

		report := checkReport(t, deps.handler(nil, nil), app.CheckRepositoryQuery{RepositoryPath: "repo", Subset: subset})

		require.True(t, report.ReadData())
		require.Equal(t, 1, report.ReadObjects())
		require.Len(t, deps.objectRepo.ReadObjectCalls(), 1)
	})
}

func TestCheckRepository_Handle_Errors(t *testing.T) {
	t.Parallel()

	errRepo := errors.New("repository failed")

	tests := []struct {
		name        string
		setup       checkSetup
		query       app.CheckRepositoryQuery
		expectedErr error
	}{
		{
			name:        "on an encrypted repository reading data without the private key, then it returns a private key required error",
			setup:       checkSetup{encryption: encryption.NewEncryptionConfig("key", "fingerprint", 1)},
			query:       app.CheckRepositoryQuery{RepositoryPath: "repo", ReadData: true},
			expectedErr: app.ErrPrivateKeyRequiredToReadData,
		},
		{
			name:        "and the manifests cannot be listed, then it returns the error",
			setup:       checkSetup{listIDsErr: errRepo},
			query:       app.CheckRepositoryQuery{RepositoryPath: "repo"},
			expectedErr: errRepo,
		},
		{
			name:        "and the objects cannot be listed, then it returns the error",
			setup:       checkSetup{listObjErr: errRepo},
			query:       app.CheckRepositoryQuery{RepositoryPath: "repo"},
			expectedErr: errRepo,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a CheckRepository query handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			deps := newCheckDependencies(t, tt.setup)

			_, err := deps.handler(nil, nil).Handle(t.Context(), tt.query)

			require.ErrorIs(t, err, tt.expectedErr)
			require.Empty(t, deps.objectRepo.ReadObjectCalls())
		})
	}
}
