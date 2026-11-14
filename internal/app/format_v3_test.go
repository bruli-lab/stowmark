package app_test

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

const (
	wrappedDataKey     = "data"
	wrappedMetadataKey = "metadata"
	generatedKey       = "generated"
)

func encryptedV3Config(t *testing.T, withMetadataKey bool) *repository.Config {
	t.Helper()
	encryptionConf := encryption.NewEncryptionConfig(wrappedDataKey, "fingerprint", 1)
	if withMetadataKey {
		encryptionConf.ChangeEncryptedMetadataKey(wrappedMetadataKey)
	}
	config := fixtures.ConfigBuilder{FormatVersion: new(repository.FormatVersionThree), EncryptionConfig: encryptionConf}.Build(t)
	return &config
}

func configRepository(config *repository.Config) *app.FolderRepositoryMock {
	return &app.FolderRepositoryMock{
		GetConfigFunc:    func(context.Context, string) (*repository.Config, error) { return config, nil },
		CreateConfigFunc: func(context.Context, string, *repository.Config) error { return nil },
	}
}

func fakeKeyRepositories() (*app.AsymmetricKeyPairRepositoryMock, *app.SymmetricKeyRepositoryMock) {
	asymmetric := &app.AsymmetricKeyPairRepositoryMock{
		ReadRSAPrivateKeyFunc:    func(context.Context, string) (*rsa.PrivateKey, error) { return &rsa.PrivateKey{}, nil },
		ReadRSAPublicKeyFunc:     func(context.Context, string) (*rsa.PublicKey, error) { return &rsa.PublicKey{}, nil },
		PublicKeyFingerPrintFunc: func(context.Context, *rsa.PublicKey) (string, error) { return "new-fingerprint", nil },
	}
	symmetric := &app.SymmetricKeyRepositoryMock{
		DecodeAndDecryptSymmetricKeyFunc: func(_ context.Context, _ *rsa.PrivateKey, encoded string) ([]byte, error) {
			return []byte("plain-" + encoded), nil
		},
		EncryptSymmetricKeyFunc: func(_ context.Context, _ *rsa.PublicKey, key []byte) ([]byte, error) {
			return []byte("wrapped-" + string(key)), nil
		},
		GenerateSymmetricKeyFunc: func(context.Context) ([]byte, error) { return []byte(generatedKey), nil },
	}
	return asymmetric, symmetric
}

func wrappedWithFakeKey(key string) string {
	return base64.StdEncoding.EncodeToString([]byte("wrapped-" + key))
}

func TestKeyring_FormatVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		config           func(t *testing.T) *repository.Config
		privateKey       *string
		expectedErr      error
		expectedDataKey  []byte
		expectedMetaKey  []byte
		encryptsMetadata bool
	}{
		{
			name:        "on an encrypted format version 3 repository without the private key, then it returns a private key required error",
			config:      func(t *testing.T) *repository.Config { return encryptedV3Config(t, true) },
			expectedErr: app.ErrPrivateKeyRequired,
		},
		{
			name:        "on an encrypted format version 3 repository without a metadata key, then it returns a missing metadata key error",
			config:      func(t *testing.T) *repository.Config { return encryptedV3Config(t, false) },
			privateKey:  new("private.pem"),
			expectedErr: app.ErrMissingMetadataKey,
		},
		{
			name:             "on an encrypted format version 3 repository with the private key, then it reads the manifest with both keys",
			config:           func(t *testing.T) *repository.Config { return encryptedV3Config(t, true) },
			privateKey:       new("private.pem"),
			expectedDataKey:  []byte("plain-" + wrappedDataKey),
			expectedMetaKey:  []byte("plain-" + wrappedMetadataKey),
			encryptsMetadata: true,
		},
		{
			name: "on an encrypted format version 2 repository without the private key, then it reads the manifest in clear",
			config: func(t *testing.T) *repository.Config {
				config := fixtures.ConfigBuilder{EncryptionConfig: encryption.NewEncryptionConfig(wrappedDataKey, "fingerprint", 1)}.Build(t)
				return &config
			},
		},
		{
			name: "on a plain format version 3 repository without the private key, then it reads the manifest in clear",
			config: func(t *testing.T) *repository.Config {
				config := fixtures.ConfigBuilder{FormatVersion: new(repository.FormatVersionThree)}.Build(t)
				return &config
			},
		},
	}
	for _, tt := range tests {
		t.Run(`Given a ManifestGet query handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			manifest := fixtures.ManifestBuilder{}.Build()
			manifestRepo := &app.ManifestRepositoryMock{
				GetFunc: func(context.Context, string, repository.Codec) (*snapshot.Manifest, error) { return &manifest, nil },
			}
			asymmetric, symmetric := fakeKeyRepositories()
			handler := app.NewManifestGet(configRepository(tt.config(t)), manifestRepo, asymmetric, symmetric)

			_, err := handler.Handle(t.Context(), app.ManifestGetQuery{RepositoryPath: "repo", SnapshotID: "id", PrivateKeyPath: tt.privateKey})

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				require.Empty(t, manifestRepo.GetCalls())
				return
			}
			require.NoError(t, err)
			require.Len(t, manifestRepo.GetCalls(), 1)
			codec := manifestRepo.GetCalls()[0].Codec
			require.Equal(t, tt.expectedDataKey, codec.DataKey())
			require.Equal(t, tt.expectedMetaKey, codec.MetadataKey())
			require.Equal(t, tt.encryptsMetadata, codec.EncryptsMetadata())
		})
	}
}

type restoreOperations struct {
	mu         sync.Mutex
	operations []string
}

func (r *restoreOperations) add(operation string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations = append(r.operations, operation)
}

func (r *restoreOperations) destination(createDirectoryErr error) *app.RestoreDestinationMock {
	return &app.RestoreDestinationMock{
		CreateDirectoryFunc: func(_ context.Context, path string) error {
			r.add("mkdir " + path)
			if createDirectoryErr != nil && strings.HasSuffix(path, "/b") {
				return createDirectoryErr
			}
			return nil
		},
		CreateSymlinkFunc: func(_ context.Context, path, target string) error {
			r.add("symlink " + path + " -> " + target)
			return nil
		},
		ApplyMetadataFunc: func(_ context.Context, path string, _ snapshot.Metadata) error {
			r.add("metadata " + path)
			return nil
		},
	}
}

func (r *restoreOperations) objectRepository() *app.ObjectRepositoryMock {
	return &app.ObjectRepositoryMock{
		RestoreObjectFunc: func(_ context.Context, file *snapshot.File, _ repository.Codec) error {
			r.add("content " + file.Path())
			return nil
		},
	}
}

func treeManifest() snapshot.Manifest {
	metadata := snapshot.NewMetadata(0o750, time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))
	entries := []snapshot.File{
		*snapshot.NewSymlink("/src/a/link", "b/file"),
		snapshot.HydrateFile("/src/a/b/file", "hash", 4, nil).WithMetadata(metadata),
		snapshot.NewDirectory("/src/a/b").WithMetadata(metadata),
		snapshot.NewDirectory("/src/a").WithMetadata(metadata),
	}
	return fixtures.ManifestBuilder{Files: entries, Source: new("/src")}.Build()
}

func TestRestoreSnapshot_Entries(t *testing.T) {
	t.Parallel()

	errMkdir := errors.New("mkdir failed")

	tests := []struct {
		name               string
		destination        *string
		createDirectoryErr error
		expectedOperations []string
		expectedFailed     int
	}{
		{
			name: "then it creates the directories, the files and the symlinks and applies the directory metadata deepest first",
			expectedOperations: []string{
				"mkdir /src/a",
				"mkdir /src/a/b",
				"content /src/a/b/file",
				"metadata /src/a/b/file",
				"symlink /src/a/link -> b/file",
				"metadata /src/a/b",
				"metadata /src/a",
			},
		},
		{
			name:        "with a destination, then it restores every entry relocated to the destination",
			destination: new("/dst"),
			expectedOperations: []string{
				"mkdir /dst/a",
				"mkdir /dst/a/b",
				"content /dst/a/b/file",
				"metadata /dst/a/b/file",
				"symlink /dst/a/link -> b/file",
				"metadata /dst/a/b",
				"metadata /dst/a",
			},
		},
		{
			name:               "and a directory cannot be created, then it reports it as failed and does not apply its metadata",
			createDirectoryErr: errMkdir,
			expectedOperations: []string{
				"mkdir /src/a",
				"mkdir /src/a/b",
				"content /src/a/b/file",
				"metadata /src/a/b/file",
				"symlink /src/a/link -> b/file",
				"metadata /src/a",
			},
			expectedFailed: 1,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a RestoreSnapshot command handler and a manifest with directories, a file and a symlink,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			manifest := treeManifest()
			operations := &restoreOperations{}
			manifestRepo := &app.ManifestRepositoryMock{
				GetFunc: func(context.Context, string, repository.Codec) (*snapshot.Manifest, error) { return &manifest, nil },
			}
			handler := app.NewRestoreSnapshot(manifestRepo, operations.objectRepository(), operations.destination(tt.createDirectoryErr),
				plainFolderRepository(t), nil, nil, newUnlockedLocker())

			events, err := handler.Handle(t.Context(), app.RestoreSnapshotCommand{RepositoryPath: "repo", SnapshotID: manifest.ID(), DestinationPath: tt.destination})

			require.NoError(t, err)
			restored, err := app.SingleEvent[*snapshot.SnapshotRestored](events)
			require.NoError(t, err)
			require.Equal(t, 4, restored.TotalFiles())
			require.Len(t, restored.Failed(), tt.expectedFailed)
			require.Equal(t, tt.expectedOperations, operations.operations)
		})
	}
}

func TestRestoreFile_Entries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		path               string
		expectedOperations []string
	}{
		{
			name:               "with a symlink, then it only creates the link",
			path:               "/src/a/link",
			expectedOperations: []string{"symlink /src/a/link -> b/file"},
		},
		{
			name:               "with a directory, then it creates it and applies its metadata",
			path:               "/src/a/b",
			expectedOperations: []string{"mkdir /src/a/b", "metadata /src/a/b"},
		},
		{
			name:               "with a regular file, then it restores its content and applies its metadata",
			path:               "/src/a/b/file",
			expectedOperations: []string{"content /src/a/b/file", "metadata /src/a/b/file"},
		},
	}
	for _, tt := range tests {
		t.Run(`Given a RestoreFile command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			manifest := treeManifest()
			operations := &restoreOperations{}
			manifestRepo := &app.ManifestRepositoryMock{
				GetFunc: func(context.Context, string, repository.Codec) (*snapshot.Manifest, error) { return &manifest, nil },
			}
			handler := app.NewRestoreFile(manifestRepo, operations.objectRepository(), operations.destination(nil),
				plainFolderRepository(t), nil, nil, newUnlockedLocker())

			_, err := handler.Handle(t.Context(), app.RestoreFileCommand{RepositoryPath: "repo", SnapshotID: manifest.ID(), FilePath: tt.path})

			require.NoError(t, err)
			require.Equal(t, tt.expectedOperations, operations.operations)
		})
	}
}

func TestInit_MetadataKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		version             repository.FormatVersion
		publicKey           *string
		existing            bool
		expectedErr         error
		expectedMetadataKey string
	}{
		{
			name:                "for a new encrypted format version 3 repository, then it stores a wrapped metadata key",
			version:             repository.FormatVersionThree,
			publicKey:           new("public.pem"),
			expectedMetadataKey: wrappedWithFakeKey(generatedKey),
		},
		{
			name:      "for a new encrypted format version 2 repository, then it stores no metadata key",
			version:   repository.FormatVersionTwo,
			publicKey: new("public.pem"),
		},
		{
			name:        "for an existing encrypted format version 3 repository without a metadata key nor a public key, then it returns a missing metadata key error",
			version:     repository.FormatVersionThree,
			existing:    true,
			expectedErr: app.ErrMissingMetadataKey,
		},
	}
	for _, tt := range tests {
		t.Run(`Given an Init command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			var previous *repository.Config
			if tt.existing {
				previous = encryptedV3Config(t, false)
			}
			folderRepo := &app.FolderRepositoryMock{
				ExistsFunc:       func(context.Context, string) (bool, error) { return tt.existing, nil },
				CreateFolderFunc: func(context.Context, string) error { return nil },
				CreateConfigFunc: func(context.Context, string, *repository.Config) error { return nil },
				GetConfigFunc:    func(context.Context, string) (*repository.Config, error) { return previous, nil },
			}
			asymmetric, symmetric := fakeKeyRepositories()
			handler := app.NewInit(folderRepo, asymmetric, symmetric, newUnlockedLocker())

			_, err := handler.Handle(t.Context(), app.InitCommand{
				RepositoryPath: "repo",
				Compression:    fixtures.CompressionBuilder{}.Build(t),
				FormatVersion:  tt.version,
				PublicKeyPath:  tt.publicKey,
			})

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				require.Empty(t, folderRepo.CreateConfigCalls())
				return
			}
			require.NoError(t, err)
			require.Len(t, folderRepo.CreateConfigCalls(), 1)
			stored := folderRepo.CreateConfigCalls()[0].C.Encryption()
			require.NotNil(t, stored)
			require.Equal(t, tt.expectedMetadataKey, stored.EncryptedMetadataKey())
		})
	}
}

func TestRewrapKey_MetadataKey(t *testing.T) {
	t.Parallel()

	t.Run(`Given a RewrapKey command handler on an encrypted format version 3 repository,
	when Handle method is called,
	then it rewraps both the data key and the metadata key with the new public key`, func(t *testing.T) {
		t.Parallel()

		config := encryptedV3Config(t, true)
		folderRepo := configRepository(config)
		asymmetric, symmetric := fakeKeyRepositories()

		_, err := app.NewRewrapKey(folderRepo, asymmetric, symmetric, newUnlockedLocker()).Handle(t.Context(), app.RewrapKeyCommand{
			RepositoryPath: "repo", OldPrivateKey: "old.pem", NewPublicKey: "new.pem",
		})

		require.NoError(t, err)
		stored := folderRepo.CreateConfigCalls()[0].C.Encryption()
		require.Equal(t, wrappedWithFakeKey("plain-"+wrappedDataKey), stored.EncryptedKey())
		require.Equal(t, wrappedWithFakeKey("plain-"+wrappedMetadataKey), stored.EncryptedMetadataKey())
		require.Equal(t, uint64(1), stored.Generation())
	})
}

func TestRekeyKey_MetadataKey(t *testing.T) {
	t.Parallel()

	t.Run(`Given a RekeyKey command handler on an encrypted format version 3 repository,
	when Handle method is called,
	then it reencrypts the objects with a new data key and keeps the same metadata key wrapped with the new public key`, func(t *testing.T) {
		t.Parallel()

		config := encryptedV3Config(t, true)
		folderRepo := configRepository(config)
		asymmetric, symmetric := fakeKeyRepositories()
		encryptedRepo := &app.EncryptedObjectRepositoryMock{
			ListEncryptedObjectsFunc: func(context.Context, uint64) ([]string, error) { return []string{"h1", "h2"}, nil },
			ReadEncryptedObjectFunc: func(context.Context, string, repository.Codec) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("object")), nil
			},
			SaveRekeyedObjectFunc:         func(context.Context, string, io.Reader, repository.Codec) error { return nil },
			DeleteEncryptedGenerationFunc: func(context.Context, uint64) error { return nil },
			AbortRekeyFunc:                func(context.Context, uint64) error { return nil },
		}

		_, err := app.NewRekeyKey(folderRepo, encryptedRepo, asymmetric, symmetric, newUnlockedLocker()).Handle(t.Context(), app.RekeyKeyCommand{
			RepositoryPath: "repo", PrivateKeyPath: "old.pem", PublicKeyPath: "new.pem",
		})

		require.NoError(t, err)
		for _, call := range encryptedRepo.ReadEncryptedObjectCalls() {
			require.Equal(t, []byte("plain-"+wrappedDataKey), call.Codec.DataKey())
			require.Equal(t, uint64(1), call.Codec.Generation())
			require.True(t, call.Codec.ContentAddressed())
		}
		require.Len(t, encryptedRepo.SaveRekeyedObjectCalls(), 2)
		for _, call := range encryptedRepo.SaveRekeyedObjectCalls() {
			require.Equal(t, []byte(generatedKey), call.Codec.DataKey())
			require.Equal(t, uint64(2), call.Codec.Generation())
		}
		stored := folderRepo.CreateConfigCalls()[0].C.Encryption()
		require.Equal(t, uint64(2), stored.Generation())
		require.Equal(t, wrappedWithFakeKey(generatedKey), stored.EncryptedKey())
		require.Equal(t, wrappedWithFakeKey("plain-"+wrappedMetadataKey), stored.EncryptedMetadataKey())
		require.Empty(t, encryptedRepo.AbortRekeyCalls())
	})
}
