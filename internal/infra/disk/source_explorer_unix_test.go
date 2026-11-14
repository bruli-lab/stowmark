//go:build unix

package disk_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/stretchr/testify/require"
)

func TestSourceExplorer_Explore_Skipped(t *testing.T) {
	t.Parallel()

	t.Run(`Given a source directory with a symbolic link and a named pipe,
	when Explore is called,
	then it keeps the symbolic link as an entry and reports the named pipe as skipped`, func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeTree(t, root, "main.go")
		require.NoError(t, os.Symlink("main.go", filepath.Join(root, "link.go")))
		require.NoError(t, syscall.Mkfifo(filepath.Join(root, "pipe"), 0o600))

		source, err := disk.NewSourceRepository().Explore(context.Background(), root, nil)

		require.NoError(t, err)
		require.ElementsMatch(t, []string{"main.go"}, explored(t, root, snapshot.RegularEntry))
		require.ElementsMatch(t, []string{"link.go"}, explored(t, root, snapshot.SymlinkEntry))
		for _, file := range source.Files() {
			if file.Kind() == snapshot.SymlinkEntry {
				require.Equal(t, "main.go", file.Target())
			}
		}
		require.Equal(t, []snapshot.SkippedFile{
			snapshot.NewSkippedFile("pipe", snapshot.NamedPipeKind),
		}, source.Skipped())
	})

	t.Run(`Given a source directory with a symbolic link,
	when Explore is called and the source is stripped to format v2,
	then the link is reported as skipped`, func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeTree(t, root, "main.go")
		require.NoError(t, os.Symlink("main.go", filepath.Join(root, "link.go")))

		source, err := disk.NewSourceRepository().Explore(context.Background(), root, nil)
		require.NoError(t, err)
		legacy := source.ForFormat(repository.FormatVersionTwo)

		require.Len(t, legacy.Files(), 1)
		require.Equal(t, []snapshot.SkippedFile{
			snapshot.NewSkippedFile("link.go", snapshot.SymlinkKind),
		}, legacy.Skipped())
	})

	t.Run(`Given a source directory with an excluded symbolic link,
	when Explore is called,
	then it does not report the excluded link as skipped`, func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeTree(t, root, "main.go")
		require.NoError(t, os.Symlink("main.go", filepath.Join(root, "link.go")))
		rules, err := snapshot.NewExcludeRules([]string{"link.go"})
		require.NoError(t, err)

		source, err := disk.NewSourceRepository().Explore(context.Background(), root, rules)

		require.NoError(t, err)
		require.Empty(t, source.Skipped())
		require.Empty(t, explored(t, root, snapshot.SymlinkEntry, "link.go"))
	})
}
