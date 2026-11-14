package app_test

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

type recordingProgress struct {
	mu         sync.Mutex
	stages     []app.ProgressStage
	totalItems int
	totalBytes int64
	items      int
	bytes      int64
	finished   int
}

func (r *recordingProgress) Start(stage app.ProgressStage, items int, bytes int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stages = append(r.stages, stage)
	r.totalItems = items
	r.totalBytes = bytes
}

func (r *recordingProgress) Advance(items int, bytes int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items += items
	r.bytes += bytes
}

func (r *recordingProgress) Finish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finished++
}

func (r *recordingProgress) requireCompleted(t *testing.T, stage app.ProgressStage, items int, bytes int64) {
	t.Helper()
	require.Equal(t, []app.ProgressStage{stage}, r.stages)
	require.Equal(t, items, r.totalItems)
	require.Equal(t, bytes, r.totalBytes)
	require.Equal(t, items, r.items, "every item must be reported once")
	require.Equal(t, bytes, r.bytes)
	require.Equal(t, 1, r.finished)
}

func totalSize(files []snapshot.File) int64 {
	var size int64
	for _, file := range files {
		size += file.Size()
	}
	return size
}

func TestProgress(t *testing.T) {
	t.Parallel()

	t.Run(`Given a CreateSnapshot command with a progress reporter,
	when Handle method is called with several workers,
	then it reports every file and its size once and finishes`, func(t *testing.T) {
		t.Parallel()

		files := sourceFiles(concurrentFiles)
		progress := &recordingProgress{}
		command := createCommand
		command.Progress = progress

		_, err := createSnapshot(t.Context(), newCreateDependencies(t, files).handler(8), &command)

		require.NoError(t, err)
		progress.requireCompleted(t, app.ProgressBackingUp, concurrentFiles, totalSize(files))
	})

	t.Run(`Given a VerifySnapshot query with a progress reporter,
	when Handle method is called,
	then it reports every file once and finishes`, func(t *testing.T) {
		t.Parallel()

		manifest, _ := verifiableManifest(concurrentFiles)
		progress := &recordingProgress{}
		query := verifyQuery
		query.Progress = progress
		objectRepo := &app.ObjectRepositoryMock{
			VerifyObjectFunc: func(context.Context, string, repository.Codec) error { return nil },
		}

		_, err := verifySnapshot(t.Context(), newVerifier(t, objectRepo, manifest), query)

		require.NoError(t, err)
		progress.requireCompleted(t, app.ProgressVerifying, concurrentFiles, totalSize(manifest.Files()))
	})

	t.Run(`Given a VerifySnapshot query with a progress reporter and a chunked file whose last chunk is missing,
	when Handle method is called,
	then it reports the bytes chunk by chunk and the whole file once it fails`, func(t *testing.T) {
		t.Parallel()

		chunks := []snapshot.Chunk{*snapshot.NewChunk("c1", 0, 4), *snapshot.NewChunk("c2", 4, 4), *snapshot.NewChunk("c3", 8, 2)}
		manifest := fixtures.ManifestBuilder{Files: []snapshot.File{
			fixtures.FileBuilder{Hash: new("file"), Size: new(int64(10)), Chunks: chunks}.Build(),
		}}.Build()
		progress := &recordingProgress{}
		query := verifyQuery
		query.Progress = progress
		objectRepo := &app.ObjectRepositoryMock{
			VerifyObjectFunc: func(_ context.Context, hash string, _ repository.Codec) error {
				if hash == "c3" {
					return snapshot.NewNotFoundError(hash)
				}
				return nil
			},
		}

		result, err := verifySnapshot(t.Context(), newVerifier(t, objectRepo, &manifest), query)

		require.NoError(t, err)
		require.False(t, result.IsSuccess())
		progress.requireCompleted(t, app.ProgressVerifying, 1, 10)
	})

	t.Run(`Given a RestoreSnapshot command with a progress reporter,
	when Handle method is called and one file fails,
	then it still reports every file once and finishes`, func(t *testing.T) {
		t.Parallel()

		manifest, _ := verifiableManifest(concurrentFiles)
		progress := &recordingProgress{}
		reads := &concurrentReads{}
		objectRepo := &app.ObjectRepositoryMock{RestoreObjectFunc: reads.restore("/source/file-003")}

		_, err := newConcurrentRestorer(t, objectRepo, manifest, plainFolderRepository(t), 8).
			Handle(t.Context(), app.RestoreSnapshotCommand{RepositoryPath: "repo", SnapshotID: manifest.ID(), Progress: progress})

		require.NoError(t, err)
		progress.requireCompleted(t, app.ProgressRestoring, concurrentFiles, totalSize(manifest.Files()))
	})

	t.Run(`Given a CheckRepository query reading data with a progress reporter,
	when Handle method is called,
	then it reports every read object and its size once and finishes`, func(t *testing.T) {
		t.Parallel()

		deps := newCheckDependencies(t, checkSetup{})
		progress := &recordingProgress{}

		report := checkReport(t, deps.handler(nil, nil), app.CheckRepositoryQuery{RepositoryPath: "repo", ReadData: true, Progress: progress})

		var bytes int64
		for _, hash := range report.ReadableObjects() {
			bytes += report.ObjectSize(hash)
		}
		progress.requireCompleted(t, app.ProgressReadingData, report.ReadObjects(), bytes)
	})

	t.Run(`Given a CheckRepository query without reading data and with a progress reporter,
	when Handle method is called,
	then it reports no progress`, func(t *testing.T) {
		t.Parallel()

		deps := newCheckDependencies(t, checkSetup{})
		progress := &recordingProgress{}

		checkReport(t, deps.handler(nil, nil), app.CheckRepositoryQuery{RepositoryPath: "repo", Progress: progress})

		require.Empty(t, progress.stages)
	})

	t.Run(`Given a RekeyKey command with a progress reporter,
	when Handle method is called,
	then it reports every rekeyed object once and finishes`, func(t *testing.T) {
		t.Parallel()

		asymmetric, symmetric := fakeKeyRepositories()
		encryptedRepo := &app.EncryptedObjectRepositoryMock{
			ListEncryptedObjectsFunc: func(context.Context, uint64) ([]string, error) { return []string{"h1", "h2", "h3"}, nil },
			ReadEncryptedObjectFunc: func(context.Context, string, repository.Codec) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("object")), nil
			},
			SaveRekeyedObjectFunc:         func(context.Context, string, io.Reader, repository.Codec) error { return nil },
			DeleteEncryptedGenerationFunc: func(context.Context, uint64) error { return nil },
			AbortRekeyFunc:                func(context.Context, uint64) error { return nil },
		}
		progress := &recordingProgress{}

		_, err := app.NewRekeyKey(configRepository(encryptedV3Config(t, true)), encryptedRepo, asymmetric, symmetric, newUnlockedLocker()).
			Handle(t.Context(), app.RekeyKeyCommand{RepositoryPath: "repo", PrivateKeyPath: "old.pem", PublicKeyPath: "new.pem", Progress: progress})

		require.NoError(t, err)
		progress.requireCompleted(t, app.ProgressRekeying, 3, 0)
	})
}
