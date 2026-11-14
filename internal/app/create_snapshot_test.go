package app_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"testing"

	"github.com/bruli-lab/stowmark/internal/app"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreateSnapshot_Handle(t *testing.T) {
	t.Parallel()
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
		source            *snapshot.Source
		hash              string
		exists            bool
		config            *repository.Config
		expectedFileCount int
		expectedTotalSize int64
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
			expectedErr: app.ErrMissingEncryptionConfigToEncrypt,
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
			name: "with error when object does not exists, then it returns a result with saved file",
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:              uuid.NewString(),
			exists:            false,
			config:            &config,
			expectedFileCount: 1,
			expectedTotalSize: int64(20),
		},
		{
			name: "with error when object does exists, then it returns a result without save files",
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:   uuid.NewString(),
			exists: true,
			config: &config,
		},
		{
			name: "with format version one and no error when object does exists, then it returns a result without saved file",
			source: new(fixtures.SourceBuilder{Files: []snapshot.File{
				fixtures.FileBuilder{}.Build(),
			}}.Build()),
			hash:   uuid.NewString(),
			exists: true,
			config: &oneConfig,
		},
		{
			name: "with format version one and encryption and no error when object does exists, then it returns without save file",
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
		t.Run(`Given a CreateSnapshot command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			sourceRepo := &app.SourceRepositoryMock{}
			sourceRepo.ExploreFunc = func(_ context.Context, _ string) (*snapshot.Source, error) {
				return tt.source, tt.exploreErr
			}
			sourceRepo.CalculateHashFunc = func(_ context.Context, _ string, _ *repository.Compression) (string, error) {
				return tt.hash, tt.calculateHashErr
			}
			manifestRepo := &app.ManifestRepositoryMock{}
			manifestRepo.SaveFunc = func(_ context.Context, _ *snapshot.Manifest) error {
				return tt.saveManifestErr
			}
			objRepo := &app.ObjectRepositoryMock{}
			objRepo.SaveFunc = func(_ context.Context, _, _ string, _ *repository.Compression, _ []byte, _ uint64) error {
				return tt.saveObjErr
			}
			objRepo.AlreadyExistsFunc = func(_ context.Context, _ string, _ []byte, _ uint64) (bool, error) {
				return tt.exists, tt.alreadyExistsErr
			}
			objRepo.SaveChunkFunc = func(_ context.Context, _, _ string, _ int64, _ int64, _ *repository.Compression, _ []byte, _ uint64) error {
				return tt.saveChunkErr
			}
			folderRepositoryRep := &app.FolderRepositoryMock{}
			folderRepositoryRep.GetConfigFunc = func(_ context.Context, _ string) (*repository.Config, error) {
				return tt.config, tt.getConfigErr
			}
			symmetricRepo := &app.SymmetricKeyRepositoryMock{}
			symmetricRepo.DecodeAndDecryptSymmetricKeyFunc = func(_ context.Context, _ *rsa.PrivateKey, _ string) ([]byte, error) {
				return []byte("abc"), tt.decodeErr
			}
			asymmetricRepo := &app.AsymmetricKeyPairRepositoryMock{}
			asymmetricRepo.ReadRSAPrivateKeyFunc = func(_ context.Context, _ string) (*rsa.PrivateKey, error) {
				return new(rsa.PrivateKey{}), nil
			}
			handler := app.NewCreateSnapshot(sourceRepo, manifestRepo, objRepo, folderRepositoryRep, asymmetricRepo, symmetricRepo, newUnlockedLocker())
			result, err := createSnapshot(t.Context(), handler, app.CreateSnapshotCommand{
				RepositoryPath: tt.args.repoPath,
				SourcePath:     tt.args.sourcePath,
				PrivateKey:     tt.args.privateKey,
			})
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, app.CreateSnapshotCommandName, result.EventName())
			require.NotEmpty(t, result.SnapshotID)
			require.Equal(t, tt.expectedFileCount, result.FileCount)
			require.Equal(t, tt.expectedTotalSize, result.TotalSize)
		})
	}
}

func createSnapshot(ctx context.Context, handler *app.CreateSnapshot, cmd app.CreateSnapshotCommand) (*app.CreateSnapshotEvent, error) {
	events, err := handler.Handle(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if len(events) != 1 {
		return nil, fmt.Errorf("unexpected number of events: %d", len(events))
	}
	result, ok := events[0].(*app.CreateSnapshotEvent)
	if !ok {
		return nil, fmt.Errorf("unexpected event type: %T", events[0])
	}
	return result, nil
}
