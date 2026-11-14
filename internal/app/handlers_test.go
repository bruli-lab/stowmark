package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

var errRepository = errors.New("repository failed")

type unknownCommand struct{}

func (unknownCommand) Name() string { return "unknown" }

type unknownQuery struct{}

func (unknownQuery) Name() string { return "unknown" }

func TestCommandHandlers_InvalidCommand(t *testing.T) {
	t.Parallel()

	handlers := map[string]cqs.CommandHandler{
		app.CreateSnapshotCommandName:  app.NewCreateSnapshot(nil, nil, nil, nil, nil, nil, nil),
		app.InitCommandName:            app.NewInit(nil, nil, nil, nil, nil),
		app.GenerateKeyCommandName:     app.NewGenerateKey(nil, nil),
		app.RekeyKeyCommandName:        app.NewRekeyKey(nil, nil, nil, nil, nil),
		app.RestoreFileCommandName:     app.NewRestoreFile(nil, nil, nil, nil, nil, nil, nil),
		app.RestoreSnapshotCommandName: app.NewRestoreSnapshot(nil, nil, nil, nil, nil, nil, nil),
		app.RewrapKeyCommandName:       app.NewRewrapKey(nil, nil, nil, nil),
		app.UnlockCommandName:          app.NewUnlock(nil),
		app.ExtendRetentionCommandName: app.NewExtendRetention(nil, nil, nil, nil, nil, nil),
	}

	for name, handler := range handlers {
		t.Run(`Given the `+name+` command handler,
		when Handle method is called with another command,
		then it returns an invalid command error`, func(t *testing.T) {
			t.Parallel()

			events, err := handler.Handle(context.Background(), unknownCommand{})

			require.ErrorAs(t, err, new(cqs.InvalidCommandError))
			require.Empty(t, events)
		})
	}
}

func TestQueryHandlers_InvalidQuery(t *testing.T) {
	t.Parallel()

	handlers := map[string]cqs.QueryHandler{
		app.ListQueryName:           app.NewSnapshotList(nil, nil, nil, nil),
		app.SnapshotGetQueryName:    app.NewManifestGet(nil, nil, nil, nil),
		app.VerifySnapshotQueryName: app.NewVerifySnapshot(nil, nil, nil, nil, nil, nil),
	}

	for name, handler := range handlers {
		t.Run(`Given the `+name+` query handler,
		when Handle method is called with another query,
		then it returns an invalid query error`, func(t *testing.T) {
			t.Parallel()

			result, err := handler.Handle(context.Background(), unknownQuery{})

			require.ErrorAs(t, err, new(cqs.InvalidQueryError))
			require.Nil(t, result)
		})
	}
}

func manifestWithFiles(t *testing.T, sizes ...int64) snapshot.Manifest {
	t.Helper()

	files := make([]snapshot.File, len(sizes))
	for index := range sizes {
		files[index] = fixtures.FileBuilder{Size: &sizes[index]}.Build()
	}
	id := "20261113T222314Z-abcdef01"
	createdAt := time.Date(2026, 11, 13, 22, 23, 14, 0, time.UTC)
	source := "/source"

	return fixtures.ManifestBuilder{ID: &id, Files: files, CreatedAt: &createdAt, Source: &source}.Build()
}

func TestSnapshotList_Handle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		list           func(ctx context.Context, codec repository.Codec) ([]snapshot.Manifest, error)
		expectedResult []snapshot.ManifestResume
		expectedErr    error
	}{
		{
			name: "and the repository fails, then it returns the repository error",
			list: func(context.Context, repository.Codec) ([]snapshot.Manifest, error) {
				return nil, errRepository
			},
			expectedErr: errRepository,
		},
		{
			name: "and the repository has manifests, then it returns their resumes",
			list: func(context.Context, repository.Codec) ([]snapshot.Manifest, error) {
				return []snapshot.Manifest{manifestWithFiles(t, 10, 20)}, nil
			},
			expectedResult: []snapshot.ManifestResume{
				*snapshot.NewManifestResume("20261113T222314Z-abcdef01", time.Date(2026, 11, 13, 22, 23, 14, 0, time.UTC), 2, 30, "/source"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(`Given a SnapshotList query handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			manifestRepo := &app.ManifestRepositoryMock{ListFunc: tt.list}
			handler := app.NewSnapshotList(plainFolderRepository(t), manifestRepo, nil, nil)

			result, err := handler.Handle(context.Background(), app.SnapshotListQuery{RepositoryPath: "repo"})

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestManifestGet_Handle(t *testing.T) {
	t.Parallel()

	t.Run(`Given a ManifestGet query handler,
	when Handle method is called with a snapshot id,
	then it returns the manifest for that id`, func(t *testing.T) {
		t.Parallel()

		manifest := manifestWithFiles(t, 5)
		manifestRepo := &app.ManifestRepositoryMock{
			GetFunc: func(context.Context, string, repository.Codec) (*snapshot.Manifest, error) {
				return &manifest, nil
			},
		}
		handler := app.NewManifestGet(plainFolderRepository(t), manifestRepo, nil, nil)

		result, err := handler.Handle(context.Background(), app.ManifestGetQuery{RepositoryPath: "repo", SnapshotID: manifest.ID()})

		require.NoError(t, err)
		require.Equal(t, &manifest, result)
		require.Len(t, manifestRepo.GetCalls(), 1)
		require.Equal(t, manifest.ID(), manifestRepo.GetCalls()[0].SnapshotID)
	})
}

type expectedRestore struct {
	totalFiles int
	failed     []snapshot.FailedResult
	isSuccess  bool
}

func TestRestoreSnapshot_Handle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		getManifest   func(ctx context.Context, snapshotID string, codec repository.Codec) (*snapshot.Manifest, error)
		restoreObject func(ctx context.Context, obj *snapshot.File, codec repository.Codec) error
		expectedEvent *expectedRestore
		expectedErr   error
	}{
		{
			name: "and the manifest cannot be read, then it returns the repository error",
			getManifest: func(context.Context, string, repository.Codec) (*snapshot.Manifest, error) {
				return nil, errRepository
			},
			expectedErr: errRepository,
		},
		{
			name: "and one file fails, then it returns an unsuccessful event with the failed file",
			getManifest: func(context.Context, string, repository.Codec) (*snapshot.Manifest, error) {
				manifest := manifestWithFiles(t, 1, 2)
				return &manifest, nil
			},
			restoreObject: func(_ context.Context, obj *snapshot.File, _ repository.Codec) error {
				if obj.Size() == 2 {
					return errRepository
				}
				return nil
			},
			expectedEvent: &expectedRestore{
				totalFiles: 2,
				failed:     []snapshot.FailedResult{*snapshot.NewFailedResult("/source/path", errRepository.Error())},
			},
		},
		{
			name: "and every file is restored, then it returns a successful event",
			getManifest: func(context.Context, string, repository.Codec) (*snapshot.Manifest, error) {
				manifest := manifestWithFiles(t, 1, 2)
				return &manifest, nil
			},
			restoreObject: func(context.Context, *snapshot.File, repository.Codec) error {
				return nil
			},
			expectedEvent: &expectedRestore{
				totalFiles: 2,
				isSuccess:  true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(`Given a RestoreSnapshot command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			manifestRepo := &app.ManifestRepositoryMock{GetFunc: tt.getManifest}
			objectRepo := &app.ObjectRepositoryMock{RestoreObjectFunc: tt.restoreObject}
			handler := app.NewRestoreSnapshot(manifestRepo, objectRepo, &app.RestoreDestinationMock{}, plainFolderRepository(t), nil, nil, newUnlockedLocker())

			events, err := handler.Handle(context.Background(), app.RestoreSnapshotCommand{RepositoryPath: "repo", SnapshotID: "20261113T222314Z-abcdef01"})

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, events, 1)
			got, ok := events[0].(*snapshot.SnapshotRestored)
			require.True(t, ok)
			require.Equal(t, snapshot.SnapshotRestoredEventName, got.EventName())
			require.Equal(t, "20261113T222314Z-abcdef01", got.SnapshotID())
			require.Equal(t, tt.expectedEvent.totalFiles, got.TotalFiles())
			require.Equal(t, tt.expectedEvent.failed, got.Failed())
			require.Equal(t, tt.expectedEvent.isSuccess, got.IsSuccess())
		})
	}
}
