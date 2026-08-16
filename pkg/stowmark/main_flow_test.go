//go:build integration

package stowmark_test

import (
	"context"
	"os"
	"testing"

	"github.com/bruli-lab/stowmark/pkg/stowmark"
	"github.com/stretchr/testify/require"
)

type Folders struct {
	Source, Repository string
	Restore            *string
}

func mainFlow(t *testing.T, ctx context.Context, folders Folders) {
	require.NoError(t, os.MkdirAll(folders.Source, 0o755))
	createSourceFixture(t, folders.Source)

	handler, err := stowmark.NewHandler(ctx, folders.Repository)
	require.NoError(t, err)
	defer func() {
		err := handler.Close()
		require.NoError(t, err)
	}()

	var created *stowmark.CreateResult

	t.Run("init repository", func(t *testing.T) {
		compressionLevel := 3
		err = handler.Init(ctx, &stowmark.Compression{
			Type:  "zstd",
			Level: &compressionLevel,
		})
		require.NoError(t, err)
	})

	t.Run("create snapshot", func(t *testing.T) {
		result, err := handler.CreateSnapshot(ctx, folders.Source)
		require.NoError(t, err)
		require.NotEmpty(t, result.ID)
		require.Equal(t, 3, result.FileCount)
		created = result
	})

	t.Run("list snapshots", func(t *testing.T) {
		snapshots, err := handler.ListSnapshots(ctx)
		require.NoError(t, err)
		require.Len(t, snapshots, 1)
		require.Equal(t, created.ID, snapshots[0].ID)
	})

	t.Run("get snapshot", func(t *testing.T) {
		snapshot, err := handler.GetSnapshot(ctx, created.ID)
		require.NoError(t, err)
		require.Equal(t, created.ID, snapshot.ID)
		require.Len(t, snapshot.Files, created.FileCount)
	})

	t.Run("verify snapshot", func(t *testing.T) {
		verification, err := handler.VerifySnapshot(ctx, created.ID)
		require.NoError(t, err)
		require.True(t, verification.IsSuccess)
		require.Empty(t, verification.Failed)
	})

	t.Run("restore snapshot", func(t *testing.T) {
		restored, err := handler.RestoreSnapshot(ctx, created.ID, folders.Restore)
		require.NoError(t, err)
		require.True(t, restored.IsSuccess)
		require.Empty(t, restored.Failed)
	})

	requireDirectoriesEqual(t, folders.Source, *folders.Restore)
}
