package app_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
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
		app.CreateSnapshotCommandName:  app.NewCreateSnapshot(nil),
		app.InitCommandName:            app.NewInit(nil),
		app.GenerateKeyCommandName:     app.NewGenerateKey(nil),
		app.RekeyKeyCommandName:        app.NewRekeyKey(nil),
		app.RestoreFileCommandName:     app.NewRestoreFile(nil),
		app.RestoreSnapshotCommandName: app.NewRestoreSnapshot(nil),
		app.RewrapKeyCommandName:       app.NewRewrapKey(nil),
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
		app.ListQueryName:           app.NewSnapshotList(nil),
		app.SnapshotGetQueryName:    app.NewManifestGet(nil),
		app.VerifySnapshotQueryName: app.NewVerifySnapshot(nil),
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
	source := "source"

	return fixtures.ManifestBuilder{ID: &id, Files: files, CreatedAt: &createdAt, Source: &source}.Build()
}

func TestSnapshotList_Handle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		list           func(ctx context.Context) ([]snapshot.Manifest, error)
		expectedResult []snapshot.ManifestResume
		expectedErr    error
	}{
		{
			name: "and the repository fails, then it returns the repository error",
			list: func(context.Context) ([]snapshot.Manifest, error) {
				return nil, errRepository
			},
			expectedErr: errRepository,
		},
		{
			name: "and the repository has manifests, then it returns their resumes",
			list: func(context.Context) ([]snapshot.Manifest, error) {
				return []snapshot.Manifest{manifestWithFiles(t, 10, 20)}, nil
			},
			expectedResult: []snapshot.ManifestResume{
				*snapshot.NewManifestResume("20261113T222314Z-abcdef01", time.Date(2026, 11, 13, 22, 23, 14, 0, time.UTC), 2, 30, "source"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(`Given a SnapshotList query handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			manifestRepo := &snapshot.ManifestRepositoryMock{ListFunc: tt.list}
			handler := app.NewSnapshotList(snapshot.NewListing(manifestRepo))

			result, err := handler.Handle(context.Background(), app.SnaphotListQuery{})

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
		manifestRepo := &snapshot.ManifestRepositoryMock{
			GetFunc: func(context.Context, string) (*snapshot.Manifest, error) {
				return &manifest, nil
			},
		}
		handler := app.NewManifestGet(snapshot.NewGetManifest(manifestRepo))

		result, err := handler.Handle(context.Background(), app.ManifestGetQuery{SnapshotID: manifest.Id()})

		require.NoError(t, err)
		require.Equal(t, &manifest, result)
		require.Len(t, manifestRepo.GetCalls(), 1)
		require.Equal(t, manifest.Id(), manifestRepo.GetCalls()[0].SnapshotID)
	})
}

func TestRestoreSnapshot_Handle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		getManifest   func(ctx context.Context, snapshotID string) (*snapshot.Manifest, error)
		restoreObject func(ctx context.Context, comp *repository.Compression, obj *snapshot.File, key []byte, generation uint64) error
		expectedEvent *app.RestoreSnapshotEvent
		expectedErr   error
	}{
		{
			name: "and the manifest cannot be read, then it returns the repository error",
			getManifest: func(context.Context, string) (*snapshot.Manifest, error) {
				return nil, errRepository
			},
			expectedErr: errRepository,
		},
		{
			name: "and one file fails, then it returns an unsuccessful event with the failed file",
			getManifest: func(context.Context, string) (*snapshot.Manifest, error) {
				manifest := manifestWithFiles(t, 1, 2)
				return &manifest, nil
			},
			restoreObject: func(_ context.Context, _ *repository.Compression, obj *snapshot.File, _ []byte, _ uint64) error {
				if obj.Size() == 2 {
					return errRepository
				}
				return nil
			},
			expectedEvent: &app.RestoreSnapshotEvent{
				SnapshotID:  "20261113T222314Z-abcdef01",
				TotalFiles:  2,
				FailedFiles: []app.Failed{{Path: "source/path", Reason: errRepository.Error()}},
			},
		},
		{
			name: "and every file is restored, then it returns a successful event",
			getManifest: func(context.Context, string) (*snapshot.Manifest, error) {
				manifest := manifestWithFiles(t, 1, 2)
				return &manifest, nil
			},
			restoreObject: func(context.Context, *repository.Compression, *snapshot.File, []byte, uint64) error {
				return nil
			},
			expectedEvent: &app.RestoreSnapshotEvent{
				SnapshotID:  "20261113T222314Z-abcdef01",
				TotalFiles:  2,
				FailedFiles: []app.Failed{},
				IsSuccess:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(`Given a RestoreSnapshot command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			manifestRepo := &snapshot.ManifestRepositoryMock{GetFunc: tt.getManifest}
			objectRepo := &snapshot.ObjectRepositoryMock{RestoreObjectFunc: tt.restoreObject}
			handler := app.NewRestoreSnapshot(snapshot.NewRestore(manifestRepo, objectRepo, nil, nil))

			events, err := handler.Handle(context.Background(), app.RestoreSnapshotCommand{SnapshotID: "20261113T222314Z-abcdef01"})

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, events, 1)
			got, ok := events[0].(*app.RestoreSnapshotEvent)
			require.True(t, ok)
			require.Equal(t, app.RestoreSnapshotCommandName, got.EventName())
			require.Equal(t, tt.expectedEvent.SnapshotID, got.SnapshotID)
			require.Equal(t, tt.expectedEvent.TotalFiles, got.TotalFiles)
			require.Equal(t, tt.expectedEvent.FailedFiles, got.FailedFiles)
			require.Equal(t, tt.expectedEvent.IsSuccess, got.IsSuccess)
		})
	}
}

func TestInit_Handle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		createErr   error
		expectedErr error
	}{
		{
			name:        "and the repository fails, then it returns the repository error",
			createErr:   errRepository,
			expectedErr: errRepository,
		},
		{
			name: "and the repository is created, then it returns an init event with the repository id",
		},
	}

	for _, tt := range tests {
		t.Run(`Given an Init command handler for a new repository,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			folderRepo := &repository.FolderRepositoryMock{
				ExistsFunc:       func(context.Context, string) (bool, error) { return false, nil },
				CreateFolderFunc: func(context.Context, string) error { return tt.createErr },
				CreateConfigFunc: func(context.Context, string, *repository.Config) error { return nil },
			}
			repo := fixtures.RepositoryBuilder{}.Build(t)
			handler := app.NewInit(repository.NewInit(folderRepo))

			events, err := handler.Handle(context.Background(), app.InitCommand{Repository: &repo})

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, events, 1)
			got, ok := events[0].(*app.InitEvent)
			require.True(t, ok)
			require.Equal(t, app.InitCommandName, got.EventName())
			require.Equal(t, repo.Config().Id().String(), got.RepositoryID)
		})
	}
}

func TestGenerateKey_Handle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		createErr   error
		expectedErr error
	}{
		{
			name:        "and the key repository fails, then it returns the repository error",
			createErr:   errRepository,
			expectedErr: errRepository,
		},
		{
			name: "and the keys are stored, then it returns no events",
		},
	}

	for _, tt := range tests {
		t.Run(`Given a GenerateKey command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			keyRepo := &encryption.AsymmetricKeyPairRepositoryMock{
				CreatePrivateKeyFunc: func(context.Context, string, *rsa.PrivateKey) error { return tt.createErr },
				CreatePublicKeyFunc:  func(context.Context, string, *rsa.PublicKey) error { return nil },
			}
			keys, err := encryption.NewAsymmetricKeyPair(t.TempDir())
			require.NoError(t, err)
			handler := app.NewGenerateKey(encryption.NewCreateAsymmetricKeyPair(keyRepo))

			events, err := handler.Handle(context.Background(), app.GenerateKeyCommand{Keys: keys})

			require.Empty(t, events)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, keyRepo.CreatePublicKeyCalls(), 1)
		})
	}
}
