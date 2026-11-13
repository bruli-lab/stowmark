package disk_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/stretchr/testify/require"
)

func TestManifestRepository_SaveAndList(t *testing.T) {
	t.Parallel()

	t.Run(`Given a ManifestRepository with a saved manifest, a leftover temporary file and a folder,
	when List method is called,
	then it returns only the saved manifest`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repositoryPath := t.TempDir()
		snapshotsPath := filepath.Join(repositoryPath, repository.SnapshotsFolder)
		require.NoError(t, os.MkdirAll(filepath.Join(snapshotsPath, "folder"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(snapshotsPath, ".partial.json.tmp-1"), []byte("{"), 0o600))

		comp, err := repository.NewCompression(repository.NoneCompressionType, nil)
		require.NoError(t, err)
		files := []snapshot.File{*snapshot.NewFile("/source/a.txt", 3)}
		manifest := snapshot.NewManifest("20261113T120000Z-abcdef12", files, time.Now().UTC(), "/source", comp)

		repo, err := newManifestRepository(repositoryPath)
		require.NoError(t, err)
		require.NoError(t, repo.Save(ctx, manifest))

		manifests, err := repo.List(ctx)

		require.NoError(t, err)
		require.Len(t, manifests, 1)
		require.Equal(t, manifest.Id(), manifests[0].Id())
	})
}
