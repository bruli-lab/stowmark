package app_test

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestInit_Handle(t *testing.T) {
	t.Parallel()

	errTest := errors.New("test error")
	previous := fixtures.ConfigBuilder{}.Build(t)
	encryptedPrevious := fixtures.ConfigBuilder{EncryptionConfig: encryption.NewEncryptionConfig("previous-key", "previous-fingerprint", uint64(3))}.Build(t)

	tests := []struct {
		name          string
		publicKeyPath *string
		force         bool
		exists        bool
		previous      *repository.Config
		expectedErr, existErr,
		createRepoErr, createConfigErr,
		createObjectErr, createSnapshotErr,
		getConfigErr, readPublicKeyErr,
		generateErr, encryptErr,
		fingerprintErr error
		expectedRepositoryID string
		expectedEncryption   *encryption.EncryptionConfig
		expectedWarnings     []string
	}{
		{
			name:        "and exists returns an error, then it returns same error",
			existErr:    errTest,
			expectedErr: errTest,
		},
		{
			name:          "and create repository folder returns an error, then it returns same error",
			createRepoErr: errTest,
			expectedErr:   errTest,
		},
		{
			name:         "for an existing repository and get config returns an error, then it returns same error",
			exists:       true,
			getConfigErr: errTest,
			expectedErr:  errTest,
		},
		{
			name:            "and create config returns an error, then it returns same error",
			createConfigErr: errTest,
			expectedErr:     errTest,
		},
		{
			name:            "and create objects folder returns an error, then it returns same error",
			createObjectErr: errTest,
			expectedErr:     errTest,
		},
		{
			name:              "and create snapshots folder returns an error, then it returns same error",
			createSnapshotErr: errTest,
			expectedErr:       errTest,
		},
		{
			name:             "with a public key that cannot be read, then it returns same error",
			publicKeyPath:    new("public.pem"),
			readPublicKeyErr: errTest,
			expectedErr:      errTest,
		},
		{
			name:          "with a public key and generate symmetric key returns an error, then it returns same error",
			publicKeyPath: new("public.pem"),
			generateErr:   errTest,
			expectedErr:   errTest,
		},
		{
			name:          "with a public key and encrypt symmetric key returns an error, then it returns same error",
			publicKeyPath: new("public.pem"),
			encryptErr:    errTest,
			expectedErr:   errTest,
		},
		{
			name:           "with a public key and fingerprint returns an error, then it returns same error",
			publicKeyPath:  new("public.pem"),
			fingerprintErr: errTest,
			expectedErr:    errTest,
		},
		{
			name: "for a new repository, then it stores a config without encryption",
		},
		{
			name:               "for a new repository with a public key, then it stores a config with the first key generation",
			publicKeyPath:      new("public.pem"),
			expectedEncryption: encryption.NewEncryptionConfig(base64.StdEncoding.EncodeToString([]byte("encrypted-key")), "fingerprint", uint64(1)),
		},
		{
			name:                 "for an existing repository, then it keeps the previous repository id",
			exists:               true,
			previous:             &previous,
			expectedRepositoryID: previous.Id().String(),
		},
		{
			name:                 "for an existing encrypted repository without public key, then it keeps the encryption and warns",
			exists:               true,
			previous:             &encryptedPrevious,
			expectedRepositoryID: encryptedPrevious.Id().String(),
			expectedEncryption:   encryptedPrevious.Encryption(),
			expectedWarnings:     []string{repository.EncryptionPreservedWarning},
		},
		{
			name:                 "for an existing encrypted repository without public key and force, then it removes the encryption and warns",
			exists:               true,
			force:                true,
			previous:             &encryptedPrevious,
			expectedRepositoryID: encryptedPrevious.Id().String(),
			expectedWarnings:     []string{repository.EncryptionRemovedWarning},
		},
	}
	for _, tt := range tests {
		t.Run(`Given an Init command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			folderRepo := &app.FolderRepositoryMock{
				ExistsFunc: func(context.Context, string) (bool, error) {
					return tt.exists, tt.existErr
				},
				CreateConfigFunc: func(context.Context, string, *repository.Config) error {
					return tt.createConfigErr
				},
				GetConfigFunc: func(context.Context, string) (*repository.Config, error) {
					return tt.previous, tt.getConfigErr
				},
			}
			folderRepo.CreateFolderFunc = func(context.Context, string) error {
				switch len(folderRepo.CreateFolderCalls()) {
				case 1:
					return tt.createRepoErr
				case 2:
					return tt.createObjectErr
				case 3:
					return tt.createSnapshotErr
				}
				return nil
			}
			asymmetricRepo := &app.AsymmetricKeyPairRepositoryMock{
				ReadRSAPublicKeyFunc: func(context.Context, string) (*rsa.PublicKey, error) {
					return &rsa.PublicKey{}, tt.readPublicKeyErr
				},
				PublicKeyFingerPrintFunc: func(context.Context, *rsa.PublicKey) (string, error) {
					return "fingerprint", tt.fingerprintErr
				},
			}
			symmetricRepo := &app.SymmetricKeyRepositoryMock{
				GenerateSymmetricKeyFunc: func(context.Context) ([]byte, error) {
					return []byte("symmetric-key"), tt.generateErr
				},
				EncryptSymmetricKeyFunc: func(context.Context, *rsa.PublicKey, []byte) ([]byte, error) {
					return []byte("encrypted-key"), tt.encryptErr
				},
			}

			handler := app.NewInit(folderRepo, asymmetricRepo, symmetricRepo)
			events, err := handler.Handle(t.Context(), app.InitCommand{
				RepositoryPath: "repo",
				Compression:    fixtures.CompressionBuilder{}.Build(t),
				FormatVersion:  repository.CurrentFormatVersion,
				PublicKeyPath:  tt.publicKeyPath,
				Force:          tt.force,
			})
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, events, 1)
			got, ok := events[0].(*app.InitEvent)
			require.True(t, ok)
			require.Equal(t, app.InitCommandName, got.EventName())
			require.Equal(t, tt.expectedWarnings, got.Warnings)

			require.Len(t, folderRepo.CreateConfigCalls(), 1)
			stored := folderRepo.CreateConfigCalls()[0].C
			require.Equal(t, stored.Id().String(), got.RepositoryID)
			if tt.expectedRepositoryID != "" {
				require.Equal(t, tt.expectedRepositoryID, got.RepositoryID)
			}
			require.Equal(t, tt.expectedEncryption, stored.Encryption())
			require.Equal(t, []string{"repo", "repo/objects", "repo/snapshots"}, []string{
				folderRepo.CreateFolderCalls()[0].Path,
				folderRepo.CreateFolderCalls()[1].Path,
				folderRepo.CreateFolderCalls()[2].Path,
			})
		})
	}
}
