//go:build integration

package stowmark_test

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const mebibyte int64 = 1024 * 1024

func createSourceFixture(t *testing.T, sourcePath string) {
	t.Helper()

	writeFixtureFile(t, sourcePath, "README.txt", []byte("Stowmark local flow test\n"))
	writeFixtureFile(t, sourcePath, "documents/notes.txt", []byte("A file inside a directory.\n"))
	writeRepeatedFixtureFile(
		t,
		sourcePath,
		"large/over-8-mib.bin",
		8*mebibyte+1,
		[]byte("stowmark-fixture-"),
	)
}

func writeFixtureFile(t *testing.T, root, relativePath string, data []byte) {
	t.Helper()

	filePath := filepath.Join(root, relativePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o755))
	require.NoError(t, os.WriteFile(filePath, data, 0o600))
}

func writeRepeatedFixtureFile(
	t *testing.T,
	root string,
	relativePath string,
	size int64,
	pattern []byte,
) {
	t.Helper()
	require.NotEmpty(t, pattern)

	filePath := filepath.Join(root, relativePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o755))

	file, err := os.Create(filePath)
	require.NoError(t, err)

	buffer := make([]byte, 64*1024)
	for i := range buffer {
		buffer[i] = pattern[i%len(pattern)]
	}

	var written int64
	for written < size {
		writeSize := min(int64(len(buffer)), size-written)
		n, writeErr := file.Write(buffer[:writeSize])
		require.NoError(t, writeErr)
		require.Equal(t, int(writeSize), n)
		written += int64(n)
	}

	require.NoError(t, file.Close())
}

func requireDirectoriesEqual(t *testing.T, expectedRoot, actualRoot string) {
	t.Helper()

	err := filepath.WalkDir(expectedRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(expectedRoot, path)
		if err != nil {
			return err
		}

		expectedHash, err := fileHash(path)
		if err != nil {
			return err
		}
		actualHash, err := fileHash(filepath.Join(actualRoot, relativePath))
		if err != nil {
			return err
		}
		if expectedHash != actualHash {
			return fmt.Errorf("restored file %q does not match the source", relativePath)
		}

		return nil
	})
	require.NoError(t, err)
}

func requireFileExists(t *testing.T, filePath string) {
	t.Helper()

	info, err := os.Stat(filePath)
	require.NoError(t, err, "expected file %q to exist", filePath)
	require.False(t, info.IsDir(), "expected %q to be a file, but it is a directory", filePath)
}

func fileHash(path string) ([sha256.Size]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer func() {
		_ = file.Close()
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return [sha256.Size]byte{}, err
	}

	return [sha256.Size]byte(hasher.Sum(nil)), nil
}
