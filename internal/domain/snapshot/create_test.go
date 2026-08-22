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

func TestCreate_Do(t *testing.T) {
	errTest := errors.New("test error")
	config := fixtures.ConfigBuilder{}.Build(t)
	oneConfig := fixtures.ConfigBuilder{FormatVersion: new(repository.FormatVersionOne)}.Build(t)
	oneConfigWithEncryption := fixtures.ConfigBuilder{
		FormatVersion:    new(repository.FormatVersionOne),
		EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
	}.Build(t)
	type args struct {
		repoPath   string
		sourcePath string
		privateKey *string
	}
	tests := []struct {
		name string
		args args
		expectedErr, getConfigErr,
		exploreErr, calculateHashErr,
		saveObjErr, alreadyExistsErr,
		saveManifestErr, saveChunkErr,
		decodeErr error
		source *snapshot.Source
		hash   string
		exists bool
		config *repository.Config
	}{
		{
			name:         "and get config returns error, then it returns same error",
			getConfigErr: errTest,
			expectedErr:  errTest,
		},
		{
			name:        "and explore returns error, then it returns same error",
			exploreErr:  errTest,
			expectedErr: errTest,
			config:      &config,
		},
		{
			name:   "and calculate hash on hash calculator returns error, then it returns same error",
			config: &config,
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			calculateHashErr: errTest,
			expectedErr:      errTest,
		},
		{
			name:             "and Already exists on saveChunk returns error, then it returns same error",
			alreadyExistsErr: errTest,
			expectedErr:      errTest,
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:   uuid.NewString(),
			exists: false,
			config: &config,
		},
		{
			name:        "with private key but no encryption config, then it returns an missing encryption config to encrypt error",
			args:        args{repoPath: "path", sourcePath: "path", privateKey: new("private-key")},
			expectedErr: snapshot.ErrMissingEncryptionConfigToEncrypt,
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:   uuid.NewString(),
			exists: false,
			config: &oneConfig,
		},
		{
			name:        "when decrypt return an error, then it returns same error",
			args:        args{repoPath: "path", sourcePath: "path", privateKey: new("private-key")},
			expectedErr: errTest,
			decodeErr:   errTest,
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:   uuid.NewString(),
			exists: false,
			config: &oneConfigWithEncryption,
		},
		{
			name:             "with format version one and Already exists on saveChunk returns error, then it returns same error",
			alreadyExistsErr: errTest,
			expectedErr:      errTest,
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:   uuid.NewString(),
			exists: false,
			config: &oneConfig,
		},
		{
			name:        "with format version one and save object on saveChunk returns error, then it returns same error",
			saveObjErr:  errTest,
			expectedErr: errTest,
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:   uuid.NewString(),
			exists: false,
			config: &oneConfig,
		},
		{
			name:         "and save chunk on saveChunk returns error, then it returns same error",
			saveChunkErr: errTest,
			expectedErr:  errTest,
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:   uuid.NewString(),
			exists: false,
			config: &config,
		},
		{
			name:            "and save manifest returns error, then it returns same error",
			saveManifestErr: errTest,
			expectedErr:     errTest,
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:   uuid.NewString(),
			exists: false,
			config: &config,
		},
		{
			name: "with error when manifest does not exists, then it returns nil",
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:   uuid.NewString(),
			exists: false,
			config: &config,
		},
		{
			name: "with error when manifest does exists, then it returns nil",
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:   uuid.NewString(),
			exists: true,
			config: &config,
		},
		{
			name: "with format version one and no error when manifest does exists, then it returns nil",
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:   uuid.NewString(),
			exists: true,
			config: &oneConfig,
		},
		{
			name: "with format version one and encryption and no error when manifest does exists, then it returns nil",
			args: args{repoPath: "path", sourcePath: "path", privateKey: new("private-key")},
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:   uuid.NewString(),
			exists: true,
			config: &oneConfigWithEncryption,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a Create service,
		when Do method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			sourceRepo := &snapshot.SourceRepositoryMock{}
			sourceRepo.ExploreFunc = func(_ context.Context, _ string) (*snapshot.Source, error) {
				return tt.source, tt.exploreErr
			}
			sourceRepo.CalculateHashFunc = func(_ context.Context, _ string, _ *repository.Compression) (string, error) {
				return tt.hash, tt.calculateHashErr
			}
			manifestRepo := &snapshot.ManifestRepositoryMock{}
			manifestRepo.SaveFunc = func(_ context.Context, _ *snapshot.Manifest) error {
				return tt.saveManifestErr
			}
			objRepo := &snapshot.ObjectRepositoryMock{}
			objRepo.SaveFunc = func(_ context.Context, _, _ string, _ *repository.Compression, _ []byte, _ uint64) error {
				return tt.saveObjErr
			}
			objRepo.AlreadyExistsFunc = func(_ context.Context, _ string, _ []byte, _ uint64) (bool, error) {
				return tt.exists, tt.alreadyExistsErr
			}
			objRepo.SaveChunkFunc = func(_ context.Context, _, _ string, _ int64, _ int64, _ *repository.Compression, _ []byte, _ uint64) error {
				return tt.saveChunkErr
			}
			folderRepositoryRep := &repository.FolderRepositoryMock{}
			folderRepositoryRep.GetConfigFunc = func(_ context.Context, _ string) (*repository.Config, error) {
				return tt.config, tt.getConfigErr
			}
			symmetricRepo := &encryption.SymmetricKeyRepositoryMock{}
			symmetricRepo.DecodeAndDecryptSymmetricKeyFunc = func(_ context.Context, _ *rsa.PrivateKey, _ string) ([]byte, error) {
				return []byte("abc"), tt.decodeErr
			}
			asymmetricRepo := &encryption.AsymmetricKeyPairRepositoryMock{}
			asymmetricRepo.ReadRSAPrivateKeyFunc = func(_ context.Context, _ string) (*rsa.PrivateKey, error) {
				return new(rsa.PrivateKey{}), nil
			}
			decryptSvc := encryption.NewDecryptSymmetricKey(symmetricRepo, asymmetricRepo)
			svc := snapshot.NewCreate(
				sourceRepo,
				manifestRepo,
				objRepo,
				repository.NewGetConfig(folderRepositoryRep),
				decryptSvc,
			)
			result, err := svc.Do(t.Context(), tt.args.repoPath, tt.args.sourcePath, tt.args.privateKey)
			if err != nil {
				require.ErrorAs(t, err, &tt.expectedErr)
				return
			}
			require.NotEmpty(t, result.Id())
			require.Equal(t, 1, result.FileCount())
			require.Equal(t, int64(20), result.TotalSize())
		})
	}
}
