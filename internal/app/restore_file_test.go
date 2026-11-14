package app_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark/internal/app"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRestoreFile_Restore(t *testing.T) {
	errTest := errors.New("test error")
	type args struct {
		filePath        string
		destinationPath *string
		privateKey      *string
	}
	files := []snapshot.File{
		fixtures.FileBuilder{Path: new("source/path1")}.Build(),
		fixtures.FileBuilder{Path: new("source/path2")}.Build(),
		fixtures.FileBuilder{Path: new("source/path3")}.Build(),
	}
	config := fixtures.ConfigBuilder{
		EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
	}.
		Build(t)
	tests := []struct {
		name string
		args args
		expectedErr, manifestErr,
		objectErr error
		manifest     *snapshot.Manifest
		config       *repository.Config
		symmetricKey []byte
		privateKey   *rsa.PrivateKey
	}{
		{
			name:        "and get manifest returns error, then it returns same error",
			manifestErr: errTest,
			expectedErr: errTest,
		},
		{
			name:        "and find file returns error, then it returns a restore file error",
			args:        args{filePath: "source/path4"},
			manifest:    new(fixtures.ManifestBuilder{Files: files}.Build()),
			expectedErr: snapshot.ErrFileNotInSnapshot,
		},
		{
			name:        "and restore object returns error, then it returns same error",
			args:        args{filePath: "source/path3"},
			manifest:    new(fixtures.ManifestBuilder{Files: files}.Build()),
			expectedErr: errTest,
			objectErr:   errTest,
		},
		{
			name:         "and restore object return nil, then it returns nil",
			args:         args{filePath: "source/path3", destinationPath: new("destinationPath")},
			manifest:     new(fixtures.ManifestBuilder{Files: files}.Build()),
			config:       &config,
			privateKey:   &rsa.PrivateKey{},
			symmetricKey: []byte("addd"),
		},
	}
	for _, tt := range tests {
		t.Run(`Given a RestoreFile command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			manifestRepo := &app.ManifestRepositoryMock{}
			manifestRepo.GetFunc = func(_ context.Context, _ string) (*snapshot.Manifest, error) {
				return tt.manifest, tt.manifestErr
			}
			objectRepo := &app.ObjectRepositoryMock{}
			objectRepo.RestoreObjectFunc = func(_ context.Context, _ *repository.Compression, _ *snapshot.File, _ []byte, _ uint64) error {
				return tt.objectErr
			}
			folderRepo := &app.FolderRepositoryMock{}
			folderRepo.GetConfigFunc = func(_ context.Context, _ string) (*repository.Config, error) {
				return tt.config, nil
			}
			symmetricRepo := &app.SymmetricKeyRepositoryMock{}
			symmetricRepo.DecodeAndDecryptSymmetricKeyFunc = func(_ context.Context, _ *rsa.PrivateKey, _ string) ([]byte, error) {
				return tt.symmetricKey, nil
			}
			asymmetricRepo := &app.AsymmetricKeyPairRepositoryMock{}
			asymmetricRepo.ReadRSAPrivateKeyFunc = func(_ context.Context, _ string) (*rsa.PrivateKey, error) {
				return tt.privateKey, nil
			}
			handler := app.NewRestoreFile(manifestRepo, objectRepo, folderRepo, asymmetricRepo, symmetricRepo, newUnlockedLocker())
			snapshotID := uuid.NewString()
			events, err := handler.Handle(t.Context(), app.RestoreFileCommand{
				SnapshotID:      snapshotID,
				FilePath:        tt.args.filePath,
				RepositoryPath:  "repository-path",
				DestinationPath: tt.args.destinationPath,
				PrivateKeyPath:  tt.args.privateKey,
			})
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, events, 1)
			got, ok := events[0].(*app.RestoreSnapshotEvent)
			require.True(t, ok)
			require.Equal(t, snapshotID, got.SnapshotID)
			require.True(t, got.IsSuccess)
		})
	}
}
