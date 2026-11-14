package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark/internal/app"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestRestoreSnapshot_HandleResult(t *testing.T) {
	t.Parallel()
	errTest := errors.New("test error")
	manifest := fixtures.ManifestBuilder{Files: []snapshot.File{
		fixtures.FileBuilder{}.Build(),
	}}.Build()
	type args struct {
		snapshotID      string
		destinationPath *string
		privateKey      *string
	}
	tests := []struct {
		name string
		args args
		expectedErr, getConfigErr,
		manifestErr, restoreErr error
		manifest          *snapshot.Manifest
		expectedSuccess   bool
		expectedFailedLen int
	}{
		{
			name:        "and get manifest returns error, then it returns same error",
			manifestErr: errTest,
			expectedErr: errTest,
		},
		{
			name:              "and restore object returns error, then it returns a failed result",
			manifest:          &manifest,
			restoreErr:        errTest,
			expectedSuccess:   false,
			expectedFailedLen: 1,
		},
		{
			name:              "with no error, then it returns same error success result",
			args:              args{snapshotID: "id", destinationPath: new("/tmp")},
			manifest:          &manifest,
			expectedSuccess:   true,
			expectedFailedLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a RestoreSnapshot command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			manifestRepo := &app.ManifestRepositoryMock{}
			manifestRepo.GetFunc = func(_ context.Context, _ string) (*snapshot.Manifest, error) {
				return tt.manifest, tt.manifestErr
			}
			objectRepo := &app.ObjectRepositoryMock{}
			objectRepo.RestoreObjectFunc = func(_ context.Context, _ *repository.Compression, _ *snapshot.File, _ []byte, _ uint64) error {
				return tt.restoreErr
			}
			folderRepo := &app.FolderRepositoryMock{}
			symmetricRepo := &app.SymmetricKeyRepositoryMock{}
			asymmetricRepo := &app.AsymmetricKeyPairRepositoryMock{}

			handler := app.NewRestoreSnapshot(manifestRepo, objectRepo, folderRepo, asymmetricRepo, symmetricRepo)
			events, err := handler.Handle(t.Context(), app.RestoreSnapshotCommand{
				SnapshotID:      tt.args.snapshotID,
				RepositoryPath:  "repository-path",
				PrivateKeyPath:  tt.args.privateKey,
				DestinationPath: tt.args.destinationPath,
			})
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, events, 1)
			result, ok := events[0].(*app.RestoreSnapshotEvent)
			require.True(t, ok)
			require.Equal(t, tt.args.snapshotID, result.SnapshotID)
			require.Equal(t, tt.expectedSuccess, result.IsSuccess)
			require.Len(t, result.FailedFiles, tt.expectedFailedLen)
		})
	}
}
