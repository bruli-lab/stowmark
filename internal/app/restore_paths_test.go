package app_test

import (
	"context"
	"testing"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func tamperedManifest(source string) *snapshot.Manifest {
	manifest := fixtures.ManifestBuilder{Source: new(source), Files: []snapshot.File{
		fixtures.FileBuilder{Path: new("/home/user/docs/a.txt"), Hash: new("aa01")}.Build(),
		fixtures.FileBuilder{Path: new("/home/user/.ssh/authorized_keys"), Hash: new("bb02")}.Build(),
		fixtures.FileBuilder{Path: new("/home/user/docs/../../../etc/cron.d/job"), Hash: new("cc03")}.Build(),
	}}.Build()
	return &manifest
}

func restoredPaths(objectRepo *app.ObjectRepositoryMock) []string {
	paths := make([]string, 0, len(objectRepo.RestoreObjectCalls()))
	for _, call := range objectRepo.RestoreObjectCalls() {
		paths = append(paths, call.File.Path())
	}
	return paths
}

func TestRestoreSnapshot_InPlacePaths(t *testing.T) {
	t.Parallel()

	t.Run(`Given a tampered manifest whose entries point outside its source,
	when RestoreSnapshot is called without a destination,
	then it only restores the entries inside the source and reports the others as failed`, func(t *testing.T) {
		t.Parallel()

		objectRepo := &app.ObjectRepositoryMock{
			RestoreObjectFunc: func(context.Context, *snapshot.File, repository.Codec) error { return nil },
		}
		manifest := tamperedManifest("/home/user/docs")

		events, err := newConcurrentRestorer(t, objectRepo, manifest, plainFolderRepository(t), 2).
			Handle(t.Context(), app.RestoreSnapshotCommand{RepositoryPath: "repo", SnapshotID: manifest.ID()})

		require.NoError(t, err)
		restored, err := app.SingleEvent[*snapshot.SnapshotRestored](events)
		require.NoError(t, err)
		require.False(t, restored.IsSuccess())
		require.Len(t, restored.Failed(), 2)
		for _, failed := range restored.Failed() {
			require.Contains(t, failed.Reason(), snapshot.ErrPathOutsideSource.Error())
		}
		require.Equal(t, []string{"/home/user/docs/a.txt"}, restoredPaths(objectRepo))
	})

	for _, source := range []string{"/", "relative/docs", "/home/user/../user/docs"} {
		t.Run(`Given a manifest whose source is `+source+`,
		when RestoreSnapshot is called without a destination,
		then it fails before writing anything`, func(t *testing.T) {
			t.Parallel()

			objectRepo := &app.ObjectRepositoryMock{}
			manifest := tamperedManifest(source)

			_, err := newConcurrentRestorer(t, objectRepo, manifest, plainFolderRepository(t), 2).
				Handle(t.Context(), app.RestoreSnapshotCommand{RepositoryPath: "repo", SnapshotID: manifest.ID()})

			require.ErrorIs(t, err, snapshot.ErrUnsafeRestoreInPlace)
			require.Empty(t, objectRepo.RestoreObjectCalls())
		})
	}

	t.Run(`Given a manifest whose source is the filesystem root,
	when RestoreSnapshot is called with a destination,
	then it restores every entry below the destination`, func(t *testing.T) {
		t.Parallel()

		objectRepo := &app.ObjectRepositoryMock{
			RestoreObjectFunc: func(context.Context, *snapshot.File, repository.Codec) error { return nil },
		}
		manifest := tamperedManifest("/")

		events, err := newConcurrentRestorer(t, objectRepo, manifest, plainFolderRepository(t), 1).
			Handle(t.Context(), app.RestoreSnapshotCommand{RepositoryPath: "repo", SnapshotID: manifest.ID(), DestinationPath: new("/restore")})

		require.NoError(t, err)
		restored, err := app.SingleEvent[*snapshot.SnapshotRestored](events)
		require.NoError(t, err)
		require.True(t, restored.IsSuccess())
		require.ElementsMatch(t, []string{"/restore/home/user/docs/a.txt", "/restore/home/user/.ssh/authorized_keys", "/restore/etc/cron.d/job"}, restoredPaths(objectRepo))
	})
}

func TestRestoreFile_InPlacePaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		source      string
		filePath    string
		expectedErr error
	}{
		{
			name:        "an entry outside the source of the manifest, when Handle method is called, then it returns a path outside source error",
			source:      "/home/user/docs",
			filePath:    "/home/user/.ssh/authorized_keys",
			expectedErr: snapshot.ErrPathOutsideSource,
		},
		{
			name:        "a manifest whose source is the filesystem root, when Handle method is called, then it returns an unsafe restore error",
			source:      "/",
			filePath:    "/home/user/docs/a.txt",
			expectedErr: snapshot.ErrUnsafeRestoreInPlace,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a RestoreFile command without a destination for `+tt.name, func(t *testing.T) {
			t.Parallel()

			manifest := tamperedManifest(tt.source)
			objectRepo := &app.ObjectRepositoryMock{}
			asymmetric, symmetric := keyRepositories()
			handler := app.NewRestoreFile(
				&app.ManifestRepositoryMock{
					GetFunc: func(context.Context, string, repository.Codec) (*snapshot.Manifest, error) { return manifest, nil },
				},
				objectRepo,
				&app.RestoreDestinationMock{},
				plainFolderRepository(t),
				asymmetric, symmetric, newUnlockedLocker(),
			)

			_, err := handler.Handle(t.Context(), app.RestoreFileCommand{RepositoryPath: "repo", SnapshotID: manifest.ID(), FilePath: tt.filePath})

			require.ErrorIs(t, err, tt.expectedErr)
			require.Empty(t, objectRepo.RestoreObjectCalls())
		})
	}
}
