package snapshot_test

import (
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/stretchr/testify/require"
)

func TestSource_ForFormat(t *testing.T) {
	t.Parallel()

	metadata := snapshot.NewMetadata(0o640, time.Now())
	source := snapshot.NewSource("/data", []snapshot.File{
		snapshot.NewDirectory("/data/docs").WithMetadata(metadata),
		snapshot.NewFile("/data/docs/file.txt", 3).WithMetadata(metadata),
		snapshot.NewSymlink("/data/docs/link", "file.txt").WithMetadata(metadata),
	}, []snapshot.SkippedFile{snapshot.NewSkippedFile("fifo", snapshot.NamedPipeKind)})

	t.Run(`Given a source with directories, symlinks and metadata,
	when ForFormat is called with version three,
	then it keeps every entry`, func(t *testing.T) {
		t.Parallel()

		got := source.ForFormat(repository.FormatVersionThree)

		require.Equal(t, source.Files(), got.Files())
		require.Equal(t, source.Skipped(), got.Skipped())
	})

	t.Run(`Given a source with directories, symlinks and metadata,
	when ForFormat is called with version two,
	then it keeps only regular files without metadata and skips the symlinks as before`, func(t *testing.T) {
		t.Parallel()

		got := source.ForFormat(repository.FormatVersionTwo)

		require.Len(t, got.Files(), 1)
		require.Equal(t, "/data/docs/file.txt", got.Files()[0].Path())
		_, hasMetadata := got.Files()[0].Metadata()
		require.False(t, hasMetadata)
		require.Equal(t, []snapshot.SkippedFile{
			snapshot.NewSkippedFile("fifo", snapshot.NamedPipeKind),
			snapshot.NewSkippedFile("docs/link", snapshot.SymlinkKind),
		}, got.Skipped())
		require.Len(t, source.Files(), 3, "the original source never changes")
	})
}
