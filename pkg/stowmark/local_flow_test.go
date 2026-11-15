//go:build integration

package stowmark_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/pkg/stowmark"
	"github.com/stretchr/testify/require"
)

func TestLocalRepositoryFlow(t *testing.T) {
	for k, version := range Versions() {
		t.Run(`Given a local repository with format version `+k+`,
		when the whole workflow runs,
		then every step succeeds`, func(t *testing.T) {
			testRoot := t.TempDir()
			sourcePath := filepath.Join(testRoot, "source-"+k)
			repositoryPath := filepath.Join(testRoot, "repository-"+k)
			restorePath := filepath.Join(testRoot, "restore-"+k)
			alternativeRestorePath := filepath.Join(testRoot, "alternative-restore-"+k)
			keyPairsFolder := filepath.Join(testRoot, "keypairs-"+k)
			newKeyPairsFolder := filepath.Join(testRoot, "new-keypairs-"+k)
			fold := Folders{
				Source:             sourcePath,
				Repository:         repositoryPath,
				Restore:            new(restorePath),
				keyPairsFolder:     keyPairsFolder,
				newKeyPairsFolder:  newKeyPairsFolder,
				AlternativeRestore: new(alternativeRestorePath),
			}
			mainFlow(t, t.Context(), fold, version)
		})
	}
}

func TestLocalRepositoryFlow_PlainFormatVersionThree(t *testing.T) {
	t.Run(`Given a plain format version 3 repository,
	when a snapshot is created, inspected, verified, checked and restored without any key,
	then the content, the metadata, the empty directories and the symlinks are restored`, func(t *testing.T) {
		ctx := t.Context()
		root := t.TempDir()
		source := filepath.Join(root, "source")
		repositoryPath := filepath.Join(root, "repository")
		restore := filepath.Join(root, "restore")
		require.NoError(t, os.MkdirAll(source, 0o755))
		createSourceFixture(t, source)
		createMetadataFixture(t, source)

		handler := openHandler(t, ctx, repositoryPath)
		require.NoError(t, handler.Init(ctx, &stowmark.Compression{Type: "lz4"}, repository.FormatVersionThree.Int(), nil, false))

		created, err := handler.CreateSnapshot(ctx, source, nil)
		require.NoError(t, err)
		require.Equal(t, 3, created.FileCount)
		require.Empty(t, created.SkippedFiles)

		manifest, err := handler.GetManifest(ctx, created.ID)
		require.NoError(t, err)
		require.Len(t, manifest.Files, expectedEntries(repository.FormatVersionThree.Int()))
		kinds := map[stowmark.EntryKind]int{}
		for _, file := range manifest.Files {
			kinds[file.Kind]++
			if file.Kind == stowmark.Symlink {
				require.Equal(t, symlinkFixtureLink, file.Target)
			}
		}
		require.Equal(t, map[stowmark.EntryKind]int{stowmark.RegularFile: 3, stowmark.Directory: 4, stowmark.Symlink: 1}, kinds)
		store := blobStore(t, ctx, repositoryPath)
		reader, err := store.Get(ctx, repository.SnapshotsFolder+"/"+created.ID+".json")
		require.NoError(t, err)
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		require.True(t, json.Valid(data), "a plain format version 3 manifest is stored in clear")

		verification, err := handler.VerifySnapshot(ctx, created.ID, nil)
		require.NoError(t, err)
		require.True(t, verification.IsSuccess)

		checked, err := handler.Check(ctx, stowmark.CheckOptions{ReadData: true})
		require.NoError(t, err)
		require.False(t, checked.HasErrors)

		restored, err := handler.RestoreSnapshot(ctx, created.ID, &restore, nil)
		require.NoError(t, err)
		require.True(t, restored.IsSuccess)
		requireDirectoriesEqual(t, source, restore)
		requireMetadataRestored(t, source, restore)

		again, err := handler.CreateSnapshot(ctx, source, nil)
		require.NoError(t, err)
		require.Zero(t, again.FileCount, "unchanged content must be deduplicated")
	})
}
