package snapshot_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"testing"

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
		fixtures.FileBuilder{Path: new("path1")}.Build(),
		fixtures.FileBuilder{Path: new("path2")}.Build(),
		fixtures.FileBuilder{Path: new("path3")}.Build(),
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
			args:        args{filePath: "path4"},
			manifest:    new(fixtures.ManifestBuilder{Files: files}.Build()),
			expectedErr: snapshot.RestoreFileError{},
		},
		{
			name:        "and restore object returns error, then it returns same error",
			args:        args{filePath: "path3"},
			manifest:    new(fixtures.ManifestBuilder{Files: files}.Build()),
			expectedErr: errTest,
			objectErr:   errTest,
		},
		{
			name:         "and restore object return nil, then it returns nil",
			args:         args{filePath: "path3", destinationPath: new("destinationPath")},
			manifest:     new(fixtures.ManifestBuilder{Files: files}.Build()),
			config:       &config,
			privateKey:   &rsa.PrivateKey{},
			symmetricKey: []byte("addd"),
		},
	}
	for _, tt := range tests {
		t.Run(`Given a RestoreFile service,
		when Restore method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			manifestRepo := &snapshot.ManifestRepositoryMock{}
			manifestRepo.GetFunc = func(_ context.Context, _ string) (*snapshot.Manifest, error) {
				return tt.manifest, tt.manifestErr
			}
			objectRepo := &snapshot.ObjectRepositoryMock{}
			objectRepo.RestoreObjectFunc = func(_ context.Context, _ *repository.Compression, _ *snapshot.File, _ []byte, _ uint64) error {
				return tt.objectErr
			}
			folderRepo := &repository.FolderRepositoryMock{}
			folderRepo.GetConfigFunc = func(_ context.Context, _ string) (*repository.Config, error) {
				return tt.config, nil
			}
			symmetricRepo := &encryption.SymmetricKeyRepositoryMock{}
			symmetricRepo.DecodeAndDecryptSymmetricKeyFunc = func(_ context.Context, _ *rsa.PrivateKey, _ string) ([]byte, error) {
				return tt.symmetricKey, nil
			}
			asymmetricRepo := &encryption.AsymmetricKeyPairRepositoryMock{}
			asymmetricRepo.ReadRSAPrivateKeyFunc = func(_ context.Context, _ string) (*rsa.PrivateKey, error) {
				return tt.privateKey, nil
			}
			svc := snapshot.NewRestoreFile(manifestRepo, objectRepo, folderRepo, encryption.NewDecryptSymmetricKey(symmetricRepo, asymmetricRepo))
			err := svc.Restore(t.Context(), uuid.NewString(), tt.args.filePath, "repository-path", tt.args.destinationPath, tt.args.privateKey)
			if err != nil {
				require.ErrorAs(t, err, &tt.expectedErr)
			}
			if tt.expectedErr != nil {
				require.Error(t, err)
			}
		})
	}
}
