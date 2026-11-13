package snapshot_test

import (
	"path/filepath"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestFile_ChangeSourcePath(t *testing.T) {
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
		when ChangeSourcePath method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			file := fixtures.FileBuilder{Path: new(filepath.FromSlash(tt.path))}.Build()

			err := file.ChangeSourcePath(source, destination)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				require.Equal(t, filepath.FromSlash(tt.path), file.Path())
				return
			}
			require.NoError(t, err)
			require.Equal(t, filepath.FromSlash(tt.expectedPath), file.Path())
		})
	}
}
