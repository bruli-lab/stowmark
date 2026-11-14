//go:build unix

package disk_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/stretchr/testify/require"
)

func TestSourceExplorer_Explore_Skipped(t *testing.T) {
	t.Parallel()

	t.Run(`Given a source directory with a symbolic link and a named pipe,
	when Explore is called,
	then it keeps only the regular files and reports the skipped ones`, func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeTree(t, root, "main.go")
		require.NoError(t, os.Symlink("main.go", filepath.Join(root, "link.go")))
		require.NoError(t, syscall.Mkfifo(filepath.Join(root, "pipe"), 0o600))

		source, err := disk.NewSourceRepository().Explore(context.Background(), root, nil)

		require.NoError(t, err)
		require.Len(t, source.Files(), 1)
		require.ElementsMatch(t, []snapshot.SkippedFile{
			snapshot.NewSkippedFile("link.go", snapshot.SymlinkKind),
			snapshot.NewSkippedFile("pipe", snapshot.NamedPipeKind),
		}, source.Skipped())
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
	})
}
