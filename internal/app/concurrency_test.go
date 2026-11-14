package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/bruli-lab/stowmark/internal/app"

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
			ExploreFunc: func(_ context.Context, _ string) (*snapshot.Source, error) {
				return &source, nil
			},
			CalculateHashFunc: func(_ context.Context, filePath string, _ *repository.Compression) (string, error) {
				return hashOf(filePath), nil
			},
		},
		manifestRepo: &app.ManifestRepositoryMock{
			SaveFunc: func(_ context.Context, _ *snapshot.Manifest) error {
				return nil
			},
		},
		objectRepo: &app.ObjectRepositoryMock{
			AlreadyExistsFunc: func(_ context.Context, _ string, _ []byte, _ uint64) (bool, error) {
				return false, nil
			},
			SaveChunkFunc: func(_ context.Context, _, _ string, _, _ int64, _ *repository.Compression, _ []byte, _ uint64) error {
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

		result, err := createSnapshot(t.Context(), deps.handler(8), createCommand)

		require.NoError(t, err)
		require.Equal(t, concurrentFiles, result.FileCount)
		require.Equal(t, int64(concurrentFiles*(concurrentFiles+1)/2), result.TotalSize)
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
		deps.objectRepo.SaveChunkFunc = func(_ context.Context, _, hash string, _, _ int64, _ *repository.Compression, _ []byte, _ uint64) error {
			if hash == failingHash {
				return errSave
			}
			return nil
		}

		result, err := createSnapshot(t.Context(), deps.handler(8), createCommand)

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
		deps.objectRepo.SaveChunkFunc = func(_ context.Context, _, _ string, _, _ int64, _ *repository.Compression, _ []byte, _ uint64) error {
			cancel()
			return nil
		}

		result, err := createSnapshot(ctx, deps.handler(4), createCommand)

		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
		require.Empty(t, deps.manifestRepo.SaveCalls())
	})
}

func newVerifier(objectRepo *app.ObjectRepositoryMock, manifest *snapshot.Manifest) *app.VerifySnapshot {
	return app.NewVerifySnapshot(
		&app.ManifestRepositoryMock{
			GetFunc: func(_ context.Context, _ string) (*snapshot.Manifest, error) {
				return manifest, nil
			},
		},
		objectRepo,
		&app.FolderRepositoryMock{},
		&app.AsymmetricKeyPairRepositoryMock{},
		&app.SymmetricKeyRepositoryMock{},
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
			ReadObjectFunc: func(_ context.Context, hash string, _ []byte, _ uint64) (io.ReadCloser, error) {
				content, ok := contents[hash]
				if !ok {
					return nil, snapshot.NewNotFoundError(hash)
				}
				return io.NopCloser(strings.NewReader(content)), nil
			},
		}

		result, err := verifySnapshot(t.Context(), newVerifier(objectRepo, manifest), verifyQuery)

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
		manifest, contents := verifiableManifest(concurrentFiles)
		objectRepo := &app.ObjectRepositoryMock{
			ReadObjectFunc: func(_ context.Context, hash string, _ []byte, _ uint64) (io.ReadCloser, error) {
				cancel()
				return io.NopCloser(strings.NewReader(contents[hash])), nil
			},
		}

		result, err := verifySnapshot(ctx, newVerifier(objectRepo, manifest), verifyQuery)

		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
	})
}
