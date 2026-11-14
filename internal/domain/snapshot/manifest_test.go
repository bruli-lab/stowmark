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
