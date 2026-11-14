package snapshot_test

import (
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestManifest_FindFile(t *testing.T) {
	t.Parallel()

	manifest := fixtures.ManifestBuilder{Files: []snapshot.File{
		fixtures.FileBuilder{Path: new("source/a")}.Build(),
		fixtures.FileBuilder{Path: new("source/b")}.Build(),
	}}.Build()

	tests := []struct {
		name        string
		filePath    string
		expectedErr error
	}{
		{
			name:        "with a path that is not in the snapshot, then it returns a file not in snapshot error",
			filePath:    "source/c",
			expectedErr: snapshot.ErrFileNotInSnapshot,
		},
		{
			name:     "with a path that is in the snapshot, then it returns that file",
			filePath: "source/b",
		},
	}
	for _, tt := range tests {
		t.Run(`Given a manifest with files,
		when FindFile method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			file, err := manifest.FindFile(tt.filePath)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				require.Nil(t, file)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.filePath, file.Path())
		})
	}
}

func TestManifest_Resume(t *testing.T) {
	t.Parallel()

	t.Run(`Given a manifest with files,
	when Resume method is called,
	then it returns the file count and the total size of its files`, func(t *testing.T) {
		t.Parallel()

		createdAt := time.Date(2026, time.November, 14, 10, 0, 0, 0, time.UTC)
		manifest := fixtures.ManifestBuilder{
			ID:        new("snapshot-id"),
			Source:    new("source"),
			CreatedAt: &createdAt,
			Files: []snapshot.File{
				fixtures.FileBuilder{Size: new(int64(10))}.Build(),
				fixtures.FileBuilder{Size: new(int64(32))}.Build(),
			},
		}.Build()

		require.Equal(t, snapshot.NewManifestResume("snapshot-id", createdAt, 2, 42, "source"), manifest.Resume())
	})
}

func TestManifest_DirectoriesAndSymlinks(t *testing.T) {
	t.Parallel()

	t.Run(`Given a manifest with a file, a directory and a symlink,
	when the referenced objects, sizes and resume are read,
	then only the regular file counts`, func(t *testing.T) {
		t.Parallel()

		manifest := fixtures.ManifestBuilder{Files: []snapshot.File{
			*snapshot.NewDirectory("/source/docs"),
			fixtures.FileBuilder{Hash: new("aa01"), Size: new(int64(5))}.Build(),
			*snapshot.NewSymlink("/source/link", "docs"),
		}}.Build()

		require.Equal(t, []string{"aa01"}, manifest.ReferencedHashes())
		require.Equal(t, map[string]int64{"aa01": 5}, manifest.ObjectSizes())
		require.Equal(t, 1, manifest.Resume().Files())
		require.Equal(t, int64(5), manifest.Resume().Size())
	})
}

func TestManifest_ValidateRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		source      string
		destination *string
		expectedErr error
	}{
		{name: "with a clean absolute source and no destination, then it can be restored in place", source: "/home/user/docs"},
		{name: "with a relative source and no destination, then it cannot be restored in place", source: "home/user", expectedErr: snapshot.ErrUnsafeRestoreInPlace},
		{name: "with a source that is not clean and no destination, then it cannot be restored in place", source: "/home/user/../../etc", expectedErr: snapshot.ErrUnsafeRestoreInPlace},
		{name: "with the filesystem root as source and no destination, then it cannot be restored in place", source: "/", expectedErr: snapshot.ErrUnsafeRestoreInPlace},
		{name: "with an empty source and no destination, then it cannot be restored in place", source: "", expectedErr: snapshot.ErrUnsafeRestoreInPlace},
		{name: "with an unsafe source and a destination, then it can be restored", source: "/", destination: new("/restore")},
	}
	for _, tt := range tests {
		t.Run(`Given a manifest `+tt.name, func(t *testing.T) {
			t.Parallel()

			manifest := fixtures.ManifestBuilder{Source: new(tt.source)}.Build()

			require.ErrorIs(t, manifest.ValidateRestore(tt.destination), tt.expectedErr)
		})
	}
}

func TestManifest_RestoreTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		path         string
		destination  *string
		expectedPath string
		expectedErr  error
	}{
		{name: "an entry inside the source and no destination, then it keeps its path", path: "/home/user/docs/a.txt", expectedPath: "/home/user/docs/a.txt"},
		{name: "the source directory itself and no destination, then it keeps its path", path: "/home/user/docs", expectedPath: "/home/user/docs"},
		{name: "an entry outside the source and no destination, then it is rejected", path: "/home/user/.ssh/authorized_keys", expectedErr: snapshot.ErrPathOutsideSource},
		{name: "an entry that escapes the source with dot dot and no destination, then it is rejected", path: "/home/user/docs/../../../etc/passwd", expectedErr: snapshot.ErrPathOutsideSource},
		{name: "an entry with a sibling prefix and no destination, then it is rejected", path: "/home/user/docs-private/key", expectedErr: snapshot.ErrPathOutsideSource},
		{name: "an entry inside the source and a destination, then it is moved below the destination", path: "/home/user/docs/sub/a.txt", destination: new("/restore"), expectedPath: "/restore/sub/a.txt"},
		{name: "an entry outside the source and a destination, then it is rejected", path: "/etc/passwd", destination: new("/restore"), expectedErr: snapshot.ErrPathOutsideSource},
	}
	for _, tt := range tests {
		t.Run(`Given a manifest of /home/user/docs and `+tt.name, func(t *testing.T) {
			t.Parallel()

			manifest := fixtures.ManifestBuilder{Source: new("/home/user/docs")}.Build()
			entry := fixtures.FileBuilder{Path: new(tt.path)}.Build()

			target, err := manifest.RestoreTarget(entry, tt.destination)

			require.ErrorIs(t, err, tt.expectedErr)
			if tt.expectedErr == nil {
				require.Equal(t, tt.expectedPath, target.Path())
			}
		})
	}
}
