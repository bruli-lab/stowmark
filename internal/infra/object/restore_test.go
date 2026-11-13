package object_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/bruli-lab/stowmark/internal/infra/object"
	"github.com/stretchr/testify/require"
)

var errPartFailed = errors.New("part failed")

func writingPart(content string) object.PartReader {
	return func(_ context.Context, _ string, destination io.Writer) (int64, error) {
		written, err := io.WriteString(destination, content)
		return int64(written), err
	}
}

func failingPart(content string) object.PartReader {
	return func(_ context.Context, _ string, destination io.Writer) (int64, error) {
		written, _ := io.WriteString(destination, content)
		return int64(written), errPartFailed
	}
}

func TestRestoreFile(t *testing.T) {
	t.Parallel()

	const previous = "previous content"
	const restored = "restored content"
	none, err := repository.NewCompression(repository.NoneCompressionType, nil)
	require.NoError(t, err)

	tests := []struct {
		name            string
		comp            *repository.Compression
		size            int64
		readPart        object.PartReader
		canceled        bool
		expectedErr     error
		expectedContent string
	}{
		{
			name:            "without compression configuration, then it returns a compression required error and keeps the existing file",
			size:            int64(len(restored)),
			readPart:        writingPart(restored),
			expectedErr:     object.ErrCompressionRequired,
			expectedContent: previous,
		},
		{
			name:            "with a canceled context, then it returns a context canceled error and keeps the existing file",
			comp:            none,
			size:            int64(len(restored)),
			readPart:        writingPart(restored),
			canceled:        true,
			expectedErr:     context.Canceled,
			expectedContent: previous,
		},
		{
			name:            "with a part that fails after writing data, then it returns the part error and keeps the existing file",
			comp:            none,
			size:            int64(len(restored)),
			readPart:        failingPart(restored),
			expectedErr:     errPartFailed,
			expectedContent: previous,
		},
		{
			name:            "with a restored size that does not match the file, then it returns a size mismatch error and keeps the existing file",
			comp:            none,
			size:            int64(len(restored)) + 1,
			readPart:        writingPart(restored),
			expectedErr:     object.ErrSizeMismatch,
			expectedContent: previous,
		},
		{
			name:            "with parts that match the file, then it replaces the existing file",
			comp:            none,
			size:            int64(len(restored)),
			readPart:        writingPart(restored),
			expectedContent: restored,
		},
	}

	for _, tt := range tests {
		t.Run(`Given an existing destination file,
		when RestoreFile function is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			directory := t.TempDir()
			destination := filepath.Join(directory, "file.txt")
			require.NoError(t, os.WriteFile(destination, []byte(previous), 0o600))
			hash := "abcdef"
			file := fixtures.FileBuilder{Path: &destination, Size: &tt.size, Hash: &hash}.Build()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.canceled {
				cancel()
			}

			err := object.RestoreFile(ctx, tt.comp, &file, tt.readPart)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
			}
			content, err := os.ReadFile(destination)
			require.NoError(t, err)
			require.Equal(t, tt.expectedContent, string(content))
			entries, err := os.ReadDir(directory)
			require.NoError(t, err)
			require.Len(t, entries, 1)
		})
	}

	t.Run(`Given no destination file,
	when RestoreFile function is called without a file,
	then it returns a file required error`, func(t *testing.T) {
		t.Parallel()

		err := object.RestoreFile(context.Background(), none, nil, writingPart(restored))

		require.ErrorIs(t, err, object.ErrFileRequired)
	})
}
