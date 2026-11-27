//go:build integration

package stowmark_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bruli-lab/stowmark/pkg/stowmark"
	"github.com/stretchr/testify/require"
)

func TestLocalRepositoryFlow_EmptyFiles(t *testing.T) {
	for k, version := range Versions() {
		for _, encrypted := range []bool{false, true} {
			mode := "plain"
			if encrypted {
				mode = "encrypted"
			}
			t.Run(`Given a `+mode+` local repository with format version `+k+` and a source with empty files,
			when a snapshot is created, verified, checked and restored,
			then every step succeeds and the empty files are restored with 0 bytes`, func(t *testing.T) {
				ctx := t.Context()
				root := t.TempDir()
				source := filepath.Join(root, "source")
				repositoryPath := filepath.Join(root, "repository")
				restore := filepath.Join(root, "restore")
				singleRestore := filepath.Join(root, "single-restore")
				writeFixtureFile(t, source, "a.txt", []byte("hola\n"))
				writeFixtureFile(t, source, "empty.txt", nil)
				writeFixtureFile(t, source, "pkg/__init__.py", nil)

				setup := openHandler(t, ctx, repositoryPath)
				var (
					publicKey  *string
					privateKey *string
					options    []stowmark.Option
				)
				if encrypted {
					keys, err := setup.KeyGenerate(ctx, filepath.Join(root, "keys"))
					require.NoError(t, err)
					publicKey = new(keys.PublicKey())
					privateKey = new(keys.PrivateKey())
					options = append(options, stowmark.WithPrivateKey(keys.PrivateKey()))
				}
				require.NoError(t, setup.Init(ctx, &stowmark.Compression{Type: "zstd"}, version, publicKey, false))
				handler := openHandler(t, ctx, repositoryPath, options...)

				created, err := handler.CreateSnapshot(ctx, source, privateKey)
				require.NoError(t, err)
				require.Equal(t, 1, created.FileCount)

				manifest, err := handler.GetManifest(ctx, created.ID)
				require.NoError(t, err)
				sizes := map[string]int64{}
				for _, file := range manifest.Files {
					if file.Kind == stowmark.RegularFile {
						sizes[file.Path] = file.Size
					}
				}
				require.Equal(t, map[string]int64{
					filepath.Join(source, "a.txt"):           5,
					filepath.Join(source, "empty.txt"):       0,
					filepath.Join(source, "pkg/__init__.py"): 0,
				}, sizes)

				verification, err := handler.VerifySnapshot(ctx, created.ID, privateKey)
				require.NoError(t, err)
				require.True(t, verification.IsSuccess)
				require.Empty(t, verification.Failed)

				checked, err := handler.Check(ctx, stowmark.CheckOptions{ReadData: true})
				require.NoError(t, err)
				require.False(t, checked.HasErrors)

				restored, err := handler.RestoreSnapshot(ctx, created.ID, &restore, privateKey)
				require.NoError(t, err)
				require.True(t, restored.IsSuccess)
				requireDirectoriesEqual(t, source, restore)
				requireEmptyFile(t, filepath.Join(restore, "empty.txt"))
				requireEmptyFile(t, filepath.Join(restore, "pkg", "__init__.py"))

				err = handler.RestoreFile(ctx, created.ID, filepath.Join(source, "empty.txt"), &singleRestore, privateKey)
				require.NoError(t, err)
				requireEmptyFile(t, filepath.Join(singleRestore, "empty.txt"))

				plan, err := handler.PlanPrune(ctx)
				require.NoError(t, err)
				require.Zero(t, plan.UnreferencedObjects)
				require.Zero(t, plan.MissingObjects)
			})
		}
	}
}

func requireEmptyFile(t *testing.T, filePath string) {
	t.Helper()

	info, err := os.Stat(filePath)
	require.NoError(t, err, "expected file %q to exist", filePath)
	require.True(t, info.Mode().IsRegular(), "expected %q to be a regular file", filePath)
	require.Zero(t, info.Size(), "expected %q to be empty", filePath)
}
