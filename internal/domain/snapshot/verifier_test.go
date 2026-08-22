package snapshot_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"io"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestVerifier_Verify(t *testing.T) {
	errTest := errors.New("test error")
	hash := uuid.NewString()
	manifest := fixtures.ManifestBuilder{Files: []snapshot.File{
		fixtures.FileBuilder{Hash: &hash}.Build(),
	}}.Build()
	type args struct {
		snapshotID     string
		repositoryPath string
		privateKey     *string
	}
	tests := []struct {
		name string
		args args
		expectedErr, getManifestErr,
		readObjectErr, getConfigErr,
		readErr error
		expectedSuccess   bool
		expectedFailedLen int
		manifest          *snapshot.Manifest
		file              *snapshot.File
		reader            io.ReadCloser
		config            *repository.Config
	}{
		{
			name:           "and get manifest returns error, then it returns same error",
			getManifestErr: errTest,
			expectedErr:    errTest,
		},
		{
			name:          "and read object returns error, then it returns same error",
			readObjectErr: errTest,
			expectedErr:   errTest,
			manifest:      &manifest,
		},
		{
			name:              "and read object returns not found error, then it returns a failed result",
			readObjectErr:     snapshot.NotFoundError{},
			manifest:          &manifest,
			expectedSuccess:   false,
			expectedFailedLen: 1,
		},
		{
			name:          "with chunks and read object returns not found error, then it returns a failed result",
			readObjectErr: snapshot.NotFoundError{},
			manifest: new(fixtures.ManifestBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{Hash: &hash, Chunks: []snapshot.Chunk{
					*snapshot.NewChunk("hash", 0, 10),
				}}.Build(),
			}}.Build()),
			expectedSuccess:   false,
			expectedFailedLen: 1,
		},
		{
			name:          "with chunks and read object returns not found error, then it returns a failed result",
			readObjectErr: snapshot.NotFoundError{},
			manifest: new(fixtures.ManifestBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{Hash: &hash, Chunks: []snapshot.Chunk{
					*snapshot.NewChunk("hash", 0, 10),
				}}.Build(),
			}}.Build()),
			expectedSuccess:   false,
			expectedFailedLen: 1,
		},
		{
			name:         "with private key, and getConfig returns error, then it returns same error",
			args:         args{privateKey: new("private-key")},
			getConfigErr: errTest,
			expectedErr:  errTest,
		},
		{
			name:        "with private key, and config has not encryption, then it returns a mission encryption config to encrypt error",
			args:        args{privateKey: new("private-key")},
			config:      new(fixtures.ConfigBuilder{}.Build(t)),
			expectedErr: snapshot.ErrMissingEncryptionConfigToEncrypt,
		},
		{
			name:        "with private key, and decrypt returns an error, then it returns same error",
			args:        args{privateKey: new("private-key")},
			config:      new(fixtures.ConfigBuilder{EncryptionConfig: encryption.NewEncryptionConfig("", "", uint64(1))}.Build(t)),
			expectedErr: errTest,
			readErr:     errTest,
		},
		{
			name:              "with chunks and he has is empty, then it returns an empty object hash failed message",
			args:              args{privateKey: new("private-key")},
			config:            new(fixtures.ConfigBuilder{EncryptionConfig: encryption.NewEncryptionConfig("", "", uint64(1))}.Build(t)),
			expectedSuccess:   false,
			expectedFailedLen: 1,
			manifest: new(fixtures.ManifestBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{Hash: &hash, Chunks: []snapshot.Chunk{
					{},
				}}.Build(),
			}}.Build()),
		},
		{
			name:              "with chunks and read object returns a not found error, then it returns an empty object hash failed message",
			args:              args{privateKey: new("private-key")},
			config:            new(fixtures.ConfigBuilder{EncryptionConfig: encryption.NewEncryptionConfig("", "", uint64(1))}.Build(t)),
			expectedSuccess:   false,
			expectedFailedLen: 1,
			manifest: new(fixtures.ManifestBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{Hash: &hash, Chunks: []snapshot.Chunk{
					*snapshot.NewChunk("hash", 0, 10),
				}}.Build(),
			}}.Build()),
			readObjectErr: snapshot.NotFoundError{},
		},
		{
			name:              "with chunks and read object returns an error, then it returns same error",
			args:              args{privateKey: new("private-key")},
			config:            new(fixtures.ConfigBuilder{EncryptionConfig: encryption.NewEncryptionConfig("", "", uint64(1))}.Build(t)),
			expectedSuccess:   false,
			expectedFailedLen: 1,
			manifest: new(fixtures.ManifestBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{Hash: &hash, Chunks: []snapshot.Chunk{
					*snapshot.NewChunk("hash", 0, 10),
				}}.Build(),
			}}.Build()),
			readObjectErr: errTest,
			expectedErr:   errTest,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a Verifier service,
		when Verify method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			objectRepo := &snapshot.ObjectRepositoryMock{}
			objectRepo.ReadObjectFunc = func(_ context.Context, _ string, _ []byte, _ uint64) (io.ReadCloser, error) {
				return tt.reader, tt.readObjectErr
			}
			manifestRepo := &snapshot.ManifestRepositoryMock{}
			manifestRepo.GetFunc = func(_ context.Context, _ string) (*snapshot.Manifest, error) {
				return tt.manifest, tt.getManifestErr
			}
			folderRepo := &repository.FolderRepositoryMock{}
			folderRepo.GetConfigFunc = func(_ context.Context, _ string) (*repository.Config, error) {
				return tt.config, tt.getConfigErr
			}
			symmetricRepo := &encryption.SymmetricKeyRepositoryMock{}
			symmetricRepo.DecodeAndDecryptSymmetricKeyFunc = func(_ context.Context, _ *rsa.PrivateKey, _ string) ([]byte, error) {
				return []byte("abc"), nil
			}
			asymmetricRepo := &encryption.AsymmetricKeyPairRepositoryMock{}
			asymmetricRepo.ReadRSAPrivateKeyFunc = func(_ context.Context, _ string) (*rsa.PrivateKey, error) {
				return new(rsa.PrivateKey{}), tt.readErr
			}
			svc := snapshot.NewVerifier(objectRepo, manifestRepo, folderRepo, encryption.NewDecryptSymmetricKey(symmetricRepo, asymmetricRepo))
			got, err := svc.Verify(t.Context(), tt.args.repositoryPath, tt.args.snapshotID, tt.args.privateKey)
			if err != nil {
				require.ErrorAs(t, err, &tt.expectedErr)
				return
			}
			if tt.expectedErr != nil {
				require.Error(t, err)
			}
			require.Equal(t, tt.expectedSuccess, got.IsSuccess())
			require.Equal(t, tt.args.snapshotID, got.SnapshotID())
			require.Len(t, got.Failed(), tt.expectedFailedLen)
		})
	}
}
