package snapshot_test

import (
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestFile_RelocatedTo(t *testing.T) {
	t.Parallel()

	source := filepath.FromSlash("/data")
	destination := filepath.FromSlash("/restore")

	tests := []struct {
		name         string
		path         string
		expectedPath string
		expectedErr  error
	}{
		{
			name:         "with a file inside the source, then it moves the file under the destination",
			path:         "/data/docs/report.txt",
			expectedPath: "/restore/docs/report.txt",
		},
		{
			name:         "with the source name repeated inside the path, then it only replaces the prefix",
			path:         "/data/backup/data/file.txt",
			expectedPath: "/restore/backup/data/file.txt",
		},
		{
			name:        "with a sibling folder that shares the source prefix, then it returns a path outside source error",
			path:        "/database/file.txt",
			expectedErr: snapshot.ErrPathOutsideSource,
		},
		{
			name:        "with a path that escapes the source, then it returns a path outside source error",
			path:        "/data/../etc/passwd",
			expectedErr: snapshot.ErrPathOutsideSource,
		},
		{
			name:        "with a file outside the source, then it returns a path outside source error",
			path:        "/other/file.txt",
			expectedErr: snapshot.ErrPathOutsideSource,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a File,
		when RelocatedTo method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			file := fixtures.FileBuilder{Path: new(filepath.FromSlash(tt.path))}.Build()

			relocated, err := file.RelocatedTo(source, destination)

			require.Equal(t, filepath.FromSlash(tt.path), file.Path(), "the original file is never changed")
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, filepath.FromSlash(tt.expectedPath), relocated.Path())
			require.Equal(t, file.Hash(), relocated.Hash())
			require.Equal(t, file.Size(), relocated.Size())
		})
	}
}

func TestFile_WithHashAndChunks(t *testing.T) {
	t.Parallel()

	t.Run(`Given a new File,
	when WithHash and WithChunks are called,
	then they return a new file with the values and leave the original untouched`, func(t *testing.T) {
		t.Parallel()

		file := *snapshot.NewFile("/data/file.bin", 8)
		chunks := []snapshot.Chunk{*snapshot.NewChunk("aa", 0, 4), *snapshot.NewChunk("bb", 4, 4)}

		hashed := file.WithHash("abc").WithChunks(chunks)
		chunks[0] = *snapshot.NewChunk("zz", 0, 4)

		require.Empty(t, file.Hash())
		require.Empty(t, file.Chunks())
		require.Equal(t, "abc", hashed.Hash())
		require.Equal(t, "aa", hashed.Chunks()[0].Hash(), "the chunks are copied")
		require.Equal(t, file.Path(), hashed.Path())
		require.Equal(t, file.Size(), hashed.Size())
	})
}

func TestHydrateFile(t *testing.T) {
	t.Parallel()

	t.Run(`Given stored file values,
	when HydrateFile is called,
	then it returns a file with every value`, func(t *testing.T) {
		t.Parallel()

		chunks := []snapshot.Chunk{*snapshot.NewChunk("aa", 0, 4)}

		file := snapshot.HydrateFile("/data/file.bin", "abc", 4, chunks)

		require.Equal(t, "/data/file.bin", file.Path())
		require.Equal(t, "abc", file.Hash())
		require.Equal(t, int64(4), file.Size())
		require.Equal(t, chunks, file.Chunks())
	})
}

func TestFile_Kinds(t *testing.T) {
	t.Parallel()

	modTime := time.Date(2026, 11, 14, 10, 0, 0, 0, time.FixedZone("CET", 3600))
	chunked := snapshot.HydrateFile("/data/big.bin", "", 8, []snapshot.Chunk{*snapshot.NewChunk("aa", 0, 4), *snapshot.NewChunk("bb", 4, 4)})

	tests := []struct {
		name           string
		file           snapshot.File
		expectedKind   snapshot.EntryKind
		expectedHashes []string
	}{
		{
			name:           "with a regular file, then it has content stored in a single object",
			file:           *snapshot.HydrateFile("/data/file.txt", "abc", 3, nil),
			expectedKind:   snapshot.RegularEntry,
			expectedHashes: []string{"abc"},
		},
		{
			name:           "with a chunked file, then its objects are the chunks",
			file:           *chunked,
			expectedKind:   snapshot.RegularEntry,
			expectedHashes: []string{"aa", "bb"},
		},
		{
			name:         "with a directory, then it has no content",
			file:         *snapshot.NewDirectory("/data/docs"),
			expectedKind: snapshot.DirectoryEntry,
		},
		{
			name:         "with a symlink, then it has no content",
			file:         *snapshot.NewSymlink("/data/link", "docs/file.txt"),
			expectedKind: snapshot.SymlinkEntry,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a File,
		when its kind and objects are read `+tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expectedKind, tt.file.Kind())
			require.Equal(t, tt.expectedKind == snapshot.RegularEntry, tt.file.HasContent())
			require.Equal(t, tt.expectedHashes, tt.file.ObjectHashes())
		})
	}

	t.Run(`Given a File with metadata,
	when the metadata is added and removed,
	then the mode keeps only permission bits, the time is in UTC and the original file never changes`, func(t *testing.T) {
		t.Parallel()

		file := *snapshot.NewSymlink("/data/link", "target")
		metadata := snapshot.NewMetadata(fs.ModeSymlink|fs.ModeSetuid|0o755, modTime)

		withMetadata := file.WithMetadata(metadata)
		got, ok := withMetadata.Metadata()

		require.True(t, ok)
		require.Equal(t, fs.ModeSetuid|0o755, got.Mode())
		require.Equal(t, time.UTC, got.ModTime().Location())
		require.True(t, modTime.Equal(got.ModTime()))
		require.Equal(t, "target", withMetadata.Target())
		_, ok = file.Metadata()
		require.False(t, ok)
		_, ok = withMetadata.WithoutMetadata().Metadata()
		require.False(t, ok)
	})
}
