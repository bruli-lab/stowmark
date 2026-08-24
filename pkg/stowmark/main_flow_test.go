//go:build integration

package stowmark_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/bruli-lab/stowmark/pkg/stowmark"
	"github.com/stretchr/testify/require"
)

type Folders struct {
	Source, Repository,
	keyPairsFolder, newKeyPairsFolder string
	Restore, AlternativeRestore *string
}

func mainFlow(t *testing.T, ctx context.Context, folders Folders, formatVersion int) {
	t.Helper()

	require.NoError(t, os.MkdirAll(folders.Source, 0o755))
	createSourceFixture(t, folders.Source)

	fileExample := fmt.Sprintf("%s/README.txt", folders.Source)

	handler, err := stowmark.NewHandler(ctx, folders.Repository)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, handler.Close())
	})

	keys, err := handler.KeyGenerate(ctx, folders.keyPairsFolder)
	require.NoError(t, err)

	compressionLevel := 3
	err = handler.Init(
		ctx,
		&stowmark.Compression{
			Type:  "zstd",
			Level: &compressionLevel,
		},
		formatVersion,
		new(keys.PublicKey()),
		false,
	)
	require.NoError(t, err)

	created, err := handler.CreateSnapshot(
		ctx,
		folders.Source,
		new(keys.PrivateKey()),
	)
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)
	require.Equal(t, 3, created.FileCount)

	snapshots, err := handler.ListSnapshots(ctx)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	require.Equal(t, created.ID, snapshots[0].ID)

	snapshot, err := handler.GetManifest(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, snapshot.ID)
	require.Len(t, snapshot.Files, created.FileCount)

	verification, err := handler.VerifySnapshot(
		ctx,
		created.ID,
		new(keys.PrivateKey()),
	)
	require.NoError(t, err)
	require.True(t, verification.IsSuccess)
	require.Empty(t, verification.Failed)

	restored, err := handler.RestoreSnapshot(
		ctx,
		created.ID,
		folders.Restore,
		new(keys.PrivateKey()),
	)
	require.NoError(t, err)
	require.True(t, restored.IsSuccess)
	require.Empty(t, restored.Failed)

	requireDirectoriesEqual(t, folders.Source, *folders.Restore)

	err = handler.RestoreFile(ctx, created.ID, fileExample, folders.AlternativeRestore, new(keys.PrivateKey()))
	require.NoError(t, err)
	requireFileExists(t, fileExample)

	newKeys, err := handler.KeyGenerate(ctx, folders.newKeyPairsFolder)
	require.NoError(t, err)
	require.NotEqual(t, keys.PublicKey(), newKeys.PublicKey())

	err = handler.Rewrap(ctx, keys.PrivateKey(), newKeys.PublicKey())
	require.NoError(t, err)

	err = handler.ReKey(ctx, newKeys.PrivateKey(), newKeys.PublicKey())
	require.NoError(t, err)
}
