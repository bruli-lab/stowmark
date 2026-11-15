//go:build integration

package stowmark_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/bruli-lab/stowmark/pkg/stowmark"
	"github.com/stretchr/testify/require"
)

const manyFiles = 256

func manyFilesFlow(t *testing.T, ctx context.Context, repositoryURL string, formatVersion int) {
	t.Helper()

	root := t.TempDir()
	source := filepath.Join(root, "source")
	for i := range manyFiles {
		writeFixtureFile(t, source, fmt.Sprintf("files/%03d/%04d.txt", i%8, i), fmt.Appendf(nil, "small file number %d of the many files flow\n", i))
	}

	setup := openHandler(t, ctx, repositoryURL)
	keys, err := setup.KeyGenerate(ctx, filepath.Join(root, "keys"))
	require.NoError(t, err)
	require.NoError(t, setup.Init(ctx, &stowmark.Compression{Type: "zstd"}, formatVersion, new(keys.PublicKey()), false))

	handler := openHandler(t, ctx, repositoryURL, stowmark.WithPrivateKey(keys.PrivateKey()))
	created, err := handler.CreateSnapshot(ctx, source, new(keys.PrivateKey()))
	require.NoError(t, err)
	require.Equal(t, manyFiles, created.FileCount)

	require.NoError(t, handler.ReKey(ctx, keys.PrivateKey(), keys.PublicKey()))

	verification, err := handler.VerifySnapshot(ctx, created.ID, new(keys.PrivateKey()))
	require.NoError(t, err)
	require.True(t, verification.IsSuccess)
	require.Empty(t, verification.Failed)

	restore := filepath.Join(root, "restore")
	restored, err := handler.RestoreSnapshot(ctx, created.ID, &restore, new(keys.PrivateKey()))
	require.NoError(t, err)
	require.True(t, restored.IsSuccess)
	requireDirectoriesEqual(t, source, restore)
}
