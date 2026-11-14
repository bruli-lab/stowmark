package localfile_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/localfile"
	"github.com/stretchr/testify/require"
)

func TestRestoreDestination_CreateDirectory(t *testing.T) {
	t.Parallel()

	t.Run(`Given a nested path,
	when CreateDirectory is called,
	then it creates every missing directory`, func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "a", "b", "c")

		require.NoError(t, localfile.NewRestoreDestination().CreateDirectory(context.Background(), path))

		info, err := os.Stat(path)
		require.NoError(t, err)
		require.True(t, info.IsDir())
	})

	t.Run(`Given a cancelled context,
	when CreateDirectory is called,
	then it returns the context error`, func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := localfile.NewRestoreDestination().CreateDirectory(ctx, filepath.Join(t.TempDir(), "a"))

		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestRestoreDestination_CreateSymlink(t *testing.T) {
	t.Parallel()

	t.Run(`Given a path whose parent does not exist,
	when CreateSymlink is called,
	then it creates the parent and the link without leaving temporaries`, func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		path := filepath.Join(root, "dir", "link")

		require.NoError(t, localfile.NewRestoreDestination().CreateSymlink(context.Background(), path, "../target"))

		target, err := os.Readlink(path)
		require.NoError(t, err)
		require.Equal(t, "../target", target)
		entries, err := os.ReadDir(filepath.Join(root, "dir"))
		require.NoError(t, err)
		require.Len(t, entries, 1)
	})

	t.Run(`Given an existing file at the path,
	when CreateSymlink is called,
	then it replaces it with the link`, func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "link")
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))

		require.NoError(t, localfile.NewRestoreDestination().CreateSymlink(context.Background(), path, "target"))

		target, err := os.Readlink(path)
		require.NoError(t, err)
		require.Equal(t, "target", target)
	})

	t.Run(`Given an existing directory at the path,
	when CreateSymlink is called,
	then it returns a directory in the way error`, func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "link")
		require.NoError(t, os.Mkdir(path, 0o755))

		err := localfile.NewRestoreDestination().CreateSymlink(context.Background(), path, "target")

		require.ErrorIs(t, err, localfile.ErrDirectoryInTheWay)
	})
}

func TestRestoreDestination_ApplyMetadata(t *testing.T) {
	t.Parallel()

	t.Run(`Given a restored file and its metadata,
	when ApplyMetadata is called,
	then it sets the permissions and the modification time`, func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))
		modTime := time.Date(2023, 5, 6, 7, 8, 9, 0, time.UTC)

		err := localfile.NewRestoreDestination().ApplyMetadata(context.Background(), path, snapshot.NewMetadata(0o640, modTime))

		require.NoError(t, err)
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o640), info.Mode().Perm())
		require.True(t, modTime.Equal(info.ModTime()))
	})

	t.Run(`Given metadata without a modification time,
	when ApplyMetadata is called,
	then it only sets the permissions`, func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))
		before, err := os.Stat(path)
		require.NoError(t, err)

		err = localfile.NewRestoreDestination().ApplyMetadata(context.Background(), path, snapshot.NewMetadata(0o644, time.Time{}))

		require.NoError(t, err)
		after, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o644), after.Mode().Perm())
		require.Equal(t, before.ModTime(), after.ModTime())
	})

	t.Run(`Given a missing path,
	when ApplyMetadata is called,
	then it returns an error`, func(t *testing.T) {
		t.Parallel()

		err := localfile.NewRestoreDestination().ApplyMetadata(context.Background(), filepath.Join(t.TempDir(), "missing"), snapshot.NewMetadata(0o644, time.Time{}))

		require.ErrorIs(t, err, os.ErrNotExist)
	})
}
