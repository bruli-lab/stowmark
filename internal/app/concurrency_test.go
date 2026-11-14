package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

const concurrentFiles = 64

func sourceFiles(count int) []snapshot.File {
	files := make([]snapshot.File, count)
	for i := range files {
		path := fmt.Sprintf("/source/file-%03d", i)
		size := int64(i + 1)
		files[i] = fixtures.FileBuilder{Path: &path, Size: &size, Hash: new("")}.Build()
	}
	return files
}

func hashOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

type createDependencies struct {
	sourceRepo   *app.SourceRepositoryMock
	manifestRepo *app.ManifestRepositoryMock
	objectRepo   *app.ObjectRepositoryMock
	folderRepo   *app.FolderRepositoryMock
}

func newCreateDependencies(t *testing.T, files []snapshot.File) createDependencies {
	t.Helper()

	config := fixtures.ConfigBuilder{}.Build(t)
	source := fixtures.SourceBuilder{Files: files}.Build()

	return createDependencies{
		sourceRepo: &app.SourceRepositoryMock{
			IgnorePatternsFunc: func(context.Context, string) ([]string, error) { return nil, nil },
			ExploreFunc: func(context.Context, string, *snapshot.ExcludeRules) (*snapshot.Source, error) {
				return &source, nil
			},
			CalculateHashFunc: func(_ context.Context, filePath string, _ repository.Codec) (string, error) {
				return hashOf(filePath), nil
			},
		},
		manifestRepo: &app.ManifestRepositoryMock{
			SaveFunc: func(_ context.Context, _ *snapshot.Manifest, _ repository.Codec) error {
				return nil
			},
		},
		objectRepo: &app.ObjectRepositoryMock{
			AlreadyExistsFunc: func(_ context.Context, _ string, _ repository.Codec) (bool, error) {
				return false, nil
			},
			SaveChunkFunc: func(_ context.Context, _ string, _ snapshot.Chunk, _ repository.Codec) error {
				return nil
			},
		},
		folderRepo: &app.FolderRepositoryMock{
			GetConfigFunc: func(_ context.Context, _ string) (*repository.Config, error) {
				return &config, nil
			},
		},
	}
}

func (d createDependencies) handler(workers int) *app.CreateSnapshot {
	return app.NewCreateSnapshotWithWorkers(
		d.sourceRepo,
		d.manifestRepo,
		d.objectRepo,
		d.folderRepo,
		&app.AsymmetricKeyPairRepositoryMock{},
		&app.SymmetricKeyRepositoryMock{},
		newUnlockedLocker(),
		workers,
	)
}

var createCommand = app.CreateSnapshotCommand{RepositoryPath: "repo", SourcePath: "source"}

func TestCreateSnapshot_Handle_Concurrency(t *testing.T) {
	t.Parallel()

	t.Run(`Given a CreateSnapshot command handler with several workers and many new files,
	when Handle method is called,
	then it saves every file, counts them all and stores a manifest with every hash`, func(t *testing.T) {
		t.Parallel()

		files := sourceFiles(concurrentFiles)
		deps := newCreateDependencies(t, files)

		result, err := createSnapshot(t.Context(), deps.handler(8), new(createCommand))

		require.NoError(t, err)
		require.Equal(t, concurrentFiles, result.FileCount())
		require.Equal(t, int64(concurrentFiles*(concurrentFiles+1)/2), result.TotalSize())
		require.Len(t, deps.objectRepo.SaveChunkCalls(), concurrentFiles)
		require.Len(t, deps.manifestRepo.SaveCalls(), 1)
		for _, file := range deps.manifestRepo.SaveCalls()[0].M.Files() {
			require.Equal(t, hashOf(file.Path()), file.Hash())
		}
	})

	t.Run(`Given a CreateSnapshot command handler with several workers and a file that fails to save,
	when Handle method is called,
	then it returns the error and does not save the manifest`, func(t *testing.T) {
		t.Parallel()

		errSave := errors.New("save failed")
		files := sourceFiles(concurrentFiles)
		deps := newCreateDependencies(t, files)
		failingHash := hashOf(files[concurrentFiles/2].Path())
		deps.objectRepo.SaveChunkFunc = func(_ context.Context, _ string, chunk snapshot.Chunk, _ repository.Codec) error {
			if chunk.Hash() == failingHash {
				return errSave
			}
			return nil
		}

		result, err := createSnapshot(t.Context(), deps.handler(8), new(createCommand))

		require.ErrorIs(t, err, errSave)
		require.Nil(t, result)
		require.Empty(t, deps.manifestRepo.SaveCalls())
	})

	t.Run(`Given a CreateSnapshot command handler with several workers,
	when the context is cancelled while files are being saved,
	then it returns the cancellation error and does not save the manifest`, func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		files := sourceFiles(concurrentFiles)
		deps := newCreateDependencies(t, files)
		deps.objectRepo.SaveChunkFunc = func(_ context.Context, _ string, _ snapshot.Chunk, _ repository.Codec) error {
			cancel()
			return nil
		}

		result, err := createSnapshot(ctx, deps.handler(4), new(createCommand))

		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
		require.Empty(t, deps.manifestRepo.SaveCalls())
	})
}

func plainFolderRepository(t *testing.T) *app.FolderRepositoryMock {
	t.Helper()
	config := fixtures.ConfigBuilder{}.Build(t)
	return &app.FolderRepositoryMock{
		GetConfigFunc: func(context.Context, string) (*repository.Config, error) { return &config, nil },
	}
}

func newVerifier(t *testing.T, objectRepo *app.ObjectRepositoryMock, manifest *snapshot.Manifest) *app.VerifySnapshot {
	t.Helper()
	return app.NewVerifySnapshot(
		&app.ManifestRepositoryMock{
			GetFunc: func(_ context.Context, _ string, _ repository.Codec) (*snapshot.Manifest, error) {
				return manifest, nil
			},
		},
		objectRepo,
		plainFolderRepository(t),
		&app.AsymmetricKeyPairRepositoryMock{},
		&app.SymmetricKeyRepositoryMock{},
		newUnlockedLocker(),
	)
}

var verifyQuery = app.VerifySnapshotQuery{RepositoryPath: "repo", SnapshotID: "snapshot"}

func verifiableManifest(count int) (manifest *snapshot.Manifest, contents map[string]string) {
	contents = make(map[string]string, count)
	files := make([]snapshot.File, count)
	for i := range files {
		content := fmt.Sprintf("content-%03d", i)
		hash := hashOf(content)
		contents[hash] = content
		files[i] = fixtures.FileBuilder{Path: new(fmt.Sprintf("/source/file-%03d", i)), Hash: &hash}.Build()
	}
	built := fixtures.ManifestBuilder{Files: files}.Build()
	return &built, contents
}

func TestVerifySnapshot_Handle_Concurrency(t *testing.T) {
	t.Parallel()

	t.Run(`Given a VerifySnapshot query handler and a manifest with many files where some objects are missing,
	when Handle method is called,
	then it checks every file and reports only the missing ones as failed`, func(t *testing.T) {
		t.Parallel()

		manifest, contents := verifiableManifest(concurrentFiles)
		missing := 0
		for hash := range contents {
			if missing == 5 {
				break
			}
			delete(contents, hash)
			missing++
		}
		objectRepo := &app.ObjectRepositoryMock{
			VerifyObjectFunc: func(_ context.Context, hash string, _ repository.Codec) error {
				if _, ok := contents[hash]; !ok {
					return snapshot.NewNotFoundError(hash)
				}
				return nil
			},
		}

		result, err := verifySnapshot(t.Context(), newVerifier(t, objectRepo, manifest), verifyQuery)

		require.NoError(t, err)
		require.Equal(t, concurrentFiles, result.TotalFiles())
		require.Len(t, result.Failed(), missing)
		require.False(t, result.IsSuccess())
	})

	t.Run(`Given a VerifySnapshot query handler and a manifest with many files,
	when the context is cancelled while objects are being read,
	then it returns the cancellation error instead of a partial result`, func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		manifest, _ := verifiableManifest(concurrentFiles)
		objectRepo := &app.ObjectRepositoryMock{
			VerifyObjectFunc: func(context.Context, string, repository.Codec) error {
				cancel()
				return nil
			},
		}

		result, err := verifySnapshot(ctx, newVerifier(t, objectRepo, manifest), verifyQuery)

		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
	})
}

func newConcurrentRestorer(t *testing.T, objectRepo *app.ObjectRepositoryMock, manifest *snapshot.Manifest, folderRepo *app.FolderRepositoryMock, workers int) *app.RestoreSnapshot {
	t.Helper()
	asymmetric, symmetric := keyRepositories()
	return app.NewRestoreSnapshotWithWorkers(
		&app.ManifestRepositoryMock{
			GetFunc: func(context.Context, string, repository.Codec) (*snapshot.Manifest, error) { return manifest, nil },
		},
		objectRepo,
		&app.RestoreDestinationMock{},
		folderRepo,
		asymmetric, symmetric, newUnlockedLocker(), workers,
	)
}

func (c *concurrentReads) restore(failing string) func(context.Context, *snapshot.File, repository.Codec) error {
	return func(_ context.Context, file *snapshot.File, _ repository.Codec) error {
		active := c.active.Add(1)
		defer c.active.Add(-1)
		for {
			current := c.maximum.Load()
			if active <= current || c.maximum.CompareAndSwap(current, active) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		if file.Path() == failing {
			return errors.New("restore failed")
		}
		return nil
	}
}

func TestRestoreSnapshot_Handle_Concurrency(t *testing.T) {
	t.Parallel()

	t.Run(`Given a RestoreSnapshot command handler with several workers and many files where one fails,
	when Handle method is called,
	then it restores the files in parallel and reports only the failed one`, func(t *testing.T) {
		t.Parallel()

		manifest, _ := verifiableManifest(concurrentFiles)
		reads := &concurrentReads{}
		objectRepo := &app.ObjectRepositoryMock{RestoreObjectFunc: reads.restore("/source/file-007")}

		events, err := newConcurrentRestorer(t, objectRepo, manifest, plainFolderRepository(t), 8).
			Handle(t.Context(), app.RestoreSnapshotCommand{RepositoryPath: "repo", SnapshotID: manifest.ID()})

		require.NoError(t, err)
		restored, err := app.SingleEvent[*snapshot.SnapshotRestored](events)
		require.NoError(t, err)
		require.Equal(t, concurrentFiles, restored.TotalFiles())
		require.Len(t, restored.Failed(), 1)
		require.Equal(t, "/source/file-007", restored.Failed()[0].Path())
		require.Len(t, objectRepo.RestoreObjectCalls(), concurrentFiles)
		require.Greater(t, reads.maximum.Load(), int32(1))
	})

	t.Run(`Given a RestoreSnapshot command handler with several workers,
	when the context is cancelled while files are being restored,
	then it returns the cancellation error instead of a partial result`, func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		manifest, _ := verifiableManifest(concurrentFiles)
		objectRepo := &app.ObjectRepositoryMock{
			RestoreObjectFunc: func(context.Context, *snapshot.File, repository.Codec) error {
				cancel()
				return nil
			},
		}

		events, err := newConcurrentRestorer(t, objectRepo, manifest, plainFolderRepository(t), 4).
			Handle(ctx, app.RestoreSnapshotCommand{RepositoryPath: "repo", SnapshotID: manifest.ID()})

		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, events)
	})

	t.Run(`Given a RestoreSnapshot command handler with several workers on an encrypted format version 2 repository,
	when Handle method is called with objects larger than the memory budget,
	then the large objects are restored one at a time`, func(t *testing.T) {
		t.Parallel()

		manifest, _ := largeManifest(t)
		config := fixtures.ConfigBuilder{EncryptionConfig: encryption.NewEncryptionConfig("key", "fingerprint", 1)}.Build(t)
		folderRepo := &app.FolderRepositoryMock{GetConfigFunc: func(context.Context, string) (*repository.Config, error) { return &config, nil }}
		reads := &concurrentReads{}
		objectRepo := &app.ObjectRepositoryMock{RestoreObjectFunc: reads.restore("")}

		_, err := newConcurrentRestorer(t, objectRepo, manifest, folderRepo, 4).
			Handle(t.Context(), app.RestoreSnapshotCommand{RepositoryPath: "repo", SnapshotID: manifest.ID(), PrivateKeyPath: new("private.pem")})

		require.NoError(t, err)
		require.Equal(t, int32(1), reads.maximum.Load())
	})
}
