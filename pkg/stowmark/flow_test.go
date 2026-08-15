package stowmark_test

import (
	"os"
	"testing"

	"github.com/bruli-lab/stowmark/pkg/stowmark"
	"github.com/stretchr/testify/require"
)

type Folders struct {
	Source, Repository string
	Restore            *string
}

func mainFlow(t *testing.T, folders Folders) {
	ctx := t.Context()
	require.NoError(t, os.MkdirAll(folders.Source, 0o755))
	createSourceFixture(t, folders.Source)

	handler, err := stowmark.NewHandler(ctx, folders.Repository)
	require.NoError(t, err)

	compressionLevel := 3
	err = handler.Init(ctx, folders.Repository, &stowmark.Compression{
		Type:  "zstd",
		Level: &compressionLevel,
	})
	require.NoError(t, err)

	created, err := handler.CreateSnapshot(ctx, folders.Source)
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)
	require.Equal(t, 3, created.FileCount)

	snapshots, err := handler.ListSnapshots(ctx)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	require.Equal(t, created.ID, snapshots[0].ID)

	snapshot, err := handler.GetSnapshot(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, snapshot.ID)
	require.Len(t, snapshot.Files, created.FileCount)

	verification, err := handler.VerifySnapshot(ctx, created.ID)
	require.NoError(t, err)
	require.True(t, verification.IsSuccess)
	require.Empty(t, verification.Failed)

	restored, err := handler.RestoreSnapshot(ctx, created.ID, folders.Restore)
	require.NoError(t, err)
	require.True(t, restored.IsSuccess)
	require.Empty(t, restored.Failed)

	requireDirectoriesEqual(t, folders.Source, *folders.Restore)
}
