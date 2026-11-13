package disk_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
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
			ID:               new(previous.Id()),
			EncryptionConfig: encryption.NewEncryptionConfig("encrypted-key", "fingerprint", 2),
		}.Build(t)

		require.NoError(t, repo.CreateConfig(ctx, repositoryPath, &updated))

		entries, err := os.ReadDir(repositoryPath)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		require.Equal(t, model.ConfigFile, entries[0].Name())

		stored, err := repo.GetConfig(ctx, repositoryPath)
		require.NoError(t, err)
		require.Equal(t, previous.Id(), stored.Id())
		require.NotNil(t, stored.Encryption())
		require.Equal(t, "encrypted-key", stored.Encryption().EncryptedKey())
		require.Equal(t, uint64(2), stored.Encryption().Generation())
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
