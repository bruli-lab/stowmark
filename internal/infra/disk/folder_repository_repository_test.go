package disk_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/stretchr/testify/require"
)

func TestFolderRepositoryRepository_CreateConfig(t *testing.T) {
	t.Parallel()

	t.Run(`Given a FolderRepositoryRepository with an existing config,
	when CreateConfig method is called with a new encryption config,
	then it replaces the config without leaving temporary files`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repositoryPath := t.TempDir()
		repo := disk.NewFolderRepositoryRepository()
		previous := fixtures.ConfigBuilder{}.Build(t)
		require.NoError(t, repo.CreateConfig(ctx, repositoryPath, &previous))

		updated := fixtures.ConfigBuilder{
			ID:               new(previous.ID()),
			EncryptionConfig: encryption.NewEncryptionConfig("encrypted-key", "fingerprint", 2),
		}.Build(t)

		require.NoError(t, repo.CreateConfig(ctx, repositoryPath, &updated))

		entries, err := os.ReadDir(repositoryPath)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		require.Equal(t, model.ConfigFile, entries[0].Name())

		stored, err := repo.GetConfig(ctx, repositoryPath)
		require.NoError(t, err)
		require.Equal(t, previous.ID(), stored.ID())
		require.NotNil(t, stored.Encryption())
		require.Equal(t, "encrypted-key", stored.Encryption().EncryptedKey())
		require.Equal(t, uint64(2), stored.Encryption().Generation())
	})

	t.Run(`Given a FolderRepositoryRepository with a config created in the past,
	when the config is read and written again,
	then it keeps the original creation date`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repositoryPath := t.TempDir()
		repo := disk.NewFolderRepositoryRepository()
		createdAt := time.Date(2025, time.March, 1, 10, 0, 0, 0, time.UTC)
		base := fixtures.ConfigBuilder{}.Build(t)
		original := repository.HydrateConfig(base.ID(), base.FormatVersion(), createdAt, base.Compression(), nil)
		require.NoError(t, repo.CreateConfig(ctx, repositoryPath, original))

		stored, err := repo.GetConfig(ctx, repositoryPath)
		require.NoError(t, err)
		require.NoError(t, repo.CreateConfig(ctx, repositoryPath, stored))
		reloaded, err := repo.GetConfig(ctx, repositoryPath)
		require.NoError(t, err)

		require.True(t, createdAt.Equal(reloaded.CreatedAt()))
	})

	t.Run(`Given a FolderRepositoryRepository and a config written by a newer format version,
	when GetConfig method is called,
	then it returns an unsupported format version error`, func(t *testing.T) {
		t.Parallel()

		repositoryPath := t.TempDir()
		config := `{"id":"8f6b0b8e-4a8e-4bb8-9a59-8a3a2f0b9c11","format_version":9,"created_at":"2026-01-01T00:00:00Z","compression":{"type":"none"}}`
		require.NoError(t, os.WriteFile(filepath.Join(repositoryPath, model.ConfigFile), []byte(config), 0o600))

		_, err := disk.NewFolderRepositoryRepository().GetConfig(context.Background(), repositoryPath)

		require.ErrorIs(t, err, repository.ErrUnsupportedFormatVersion)
	})

	t.Run(`Given a FolderRepositoryRepository and a repository path that does not exist,
	when CreateConfig method is called,
	then it returns an error and does not create the config`, func(t *testing.T) {
		t.Parallel()

		repositoryPath := filepath.Join(t.TempDir(), "missing")
		config := fixtures.ConfigBuilder{}.Build(t)

		err := disk.NewFolderRepositoryRepository().CreateConfig(context.Background(), repositoryPath, &config)

		require.Error(t, err)
		require.NoFileExists(t, filepath.Join(repositoryPath, model.ConfigFile))
	})
}
