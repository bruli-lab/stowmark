package localfile_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bruli-lab/stowmark/internal/infra/localfile"
	"github.com/stretchr/testify/require"
)

func TestWriteAtomically(t *testing.T) {
	t.Parallel()

	t.Run(`Given a directory,
	when WriteBytesAtomically is called,
	then the file has the content and the permissions and no temporary file is left`, func(t *testing.T) {
		t.Parallel()

		directory := t.TempDir()
		target := filepath.Join(directory, "config.json")

		require.NoError(t, localfile.WriteBytesAtomically(target, 0o600, []byte("content")))

		content, err := os.ReadFile(target)
		require.NoError(t, err)
		require.Equal(t, "content", string(content))
		if runtime.GOOS != "windows" {
			info, err := os.Stat(target)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		}
		entries, err := os.ReadDir(directory)
		require.NoError(t, err)
		require.Len(t, entries, 1)
	})

	t.Run(`Given an existing file,
	when WriteAtomically is called,
	then it replaces the whole content`, func(t *testing.T) {
		t.Parallel()

		target := filepath.Join(t.TempDir(), "config.json")
		require.NoError(t, os.WriteFile(target, []byte("a much longer previous content"), 0o600))

		require.NoError(t, localfile.WriteBytesAtomically(target, 0o600, []byte("new")))

		content, err := os.ReadFile(target)
		require.NoError(t, err)
		require.Equal(t, "new", string(content))
	})

	t.Run(`Given a write function that fails after writing part of the data,
	when WriteAtomically is called,
	then it returns the error, keeps the previous file and removes the temporary file`, func(t *testing.T) {
		t.Parallel()

		errWrite := errors.New("write failed")
		directory := t.TempDir()
		target := filepath.Join(directory, "config.json")
		require.NoError(t, os.WriteFile(target, []byte("previous"), 0o600))

		err := localfile.WriteAtomically(target, 0o600, func(w io.Writer) error {
			_, _ = w.Write([]byte("partial"))
			return errWrite
		})

		require.ErrorIs(t, err, errWrite)
		content, err := os.ReadFile(target)
		require.NoError(t, err)
		require.Equal(t, "previous", string(content))
		entries, err := os.ReadDir(directory)
		require.NoError(t, err)
		require.Len(t, entries, 1)
	})

	t.Run(`Given a directory that does not exist,
	when WriteAtomically is called,
	then it returns an error`, func(t *testing.T) {
		t.Parallel()

		target := filepath.Join(t.TempDir(), "missing", "config.json")

		err := localfile.WriteBytesAtomically(target, 0o600, []byte("content"))

		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run(`Given a target that is a directory,
	when WriteAtomically is called,
	then it fails to commit and removes the temporary file`, func(t *testing.T) {
		t.Parallel()

		directory := t.TempDir()
		target := filepath.Join(directory, "taken")
		require.NoError(t, os.MkdirAll(filepath.Join(target, "child"), 0o750))

		err := localfile.WriteBytesAtomically(target, 0o600, []byte("content"))

		require.ErrorContains(t, err, "commit file")
		entries, err := os.ReadDir(directory)
		require.NoError(t, err)
		require.Len(t, entries, 1)
	})
}

func TestIsTemporary(t *testing.T) {
	t.Parallel()

	t.Run(`Given file names,
	when IsTemporary is called,
	then only the names with the temporary prefix are temporary`, func(t *testing.T) {
		t.Parallel()

		require.True(t, localfile.IsTemporary(".config.json.tmp-123"))
		require.False(t, localfile.IsTemporary("config.json"))
	})
}
