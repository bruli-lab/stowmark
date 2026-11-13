package disk_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/object"
	"github.com/stretchr/testify/require"
)

func writeSourceFile(t *testing.T, data []byte) string {
	t.Helper()

	filePath := filepath.Join(t.TempDir(), "source.bin")
	require.NoError(t, os.WriteFile(filePath, data, 0o600))

	return filePath
}

func compressedHash(t *testing.T, comp *repository.Compression, data []byte) string {
	t.Helper()

	handler, err := compression.NewHandlersFactory().GetHandler(comp.CompType())
	require.NoError(t, err)

	hasher := sha256.New()
	encoder, err := handler.Encode(hasher, comp.Level())
	require.NoError(t, err)
	_, err = encoder.Writer.Write(data)
	require.NoError(t, err)
	require.NoError(t, encoder.Closer())

	return hex.EncodeToString(hasher.Sum(nil))
}

func newSymmetricKey(t *testing.T) []byte {
	t.Helper()

	key := make([]byte, encrypt.AES256KeySize)
	_, err := rand.Read(key)
	require.NoError(t, err)

	return key
}

func repositoryFiles(t *testing.T, root string) []string {
	t.Helper()

	var files []string
	err := filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			files = append(files, filePath)
		}
		return nil
	})
	require.NoError(t, err)

	return files
}

func TestObjectRepository_Save(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte("stowmark "), 2048)
	comp, err := repository.NewCompression(repository.ZstdCompressionType, nil)
	require.NoError(t, err)
	hash := compressedHash(t, comp, data)

	tests := []struct {
		name        string
		hash        string
		encrypted   bool
		expectedErr error
		expectSaved bool
	}{
		{
			name:        "with a hash that does not match the source, then it returns an error and leaves no files",
			hash:        strings.Repeat("ab", sha256.Size),
			expectedErr: object.ErrHashMismatch,
		},
		{
			name:        "with encryption and a hash that does not match the source, then it returns an error and leaves no files",
			hash:        strings.Repeat("ab", sha256.Size),
			encrypted:   true,
			expectedErr: object.ErrHashMismatch,
		},
		{
			name:        "with the source hash, then it stores the object",
			hash:        hash,
			expectSaved: true,
		},
		{
			name:        "with encryption and the source hash, then it stores the object",
			hash:        hash,
			encrypted:   true,
			expectSaved: true,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a disk ObjectRepository,
		when Save method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			repositoryPath := t.TempDir()
			sourcePath := writeSourceFile(t, data)

			var key []byte
			if tt.encrypted {
				key = newSymmetricKey(t)
			}

			repo, err := disk.NewObjectRepository(repositoryPath)
			require.NoError(t, err)

			err = repo.Save(ctx, sourcePath, tt.hash, comp, key, 1)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				require.Empty(t, repositoryFiles(t, repositoryPath))

				exists, existsErr := repo.AlreadyExists(ctx, tt.hash, key, 1)
				require.NoError(t, existsErr)
				require.False(t, exists)
				return
			}

			require.NoError(t, err)
			require.Len(t, repositoryFiles(t, repositoryPath), 1)

			exists, err := repo.AlreadyExists(ctx, tt.hash, key, 1)
			require.NoError(t, err)
			require.True(t, exists)

			reader, err := repo.ReadObject(ctx, tt.hash, key, 1)
			require.NoError(t, err)
			defer func() {
				_ = reader.Close()
			}()
			stored, err := io.ReadAll(reader)
			require.NoError(t, err)
			calculated := sha256.Sum256(stored)
			require.Equal(t, tt.hash, hex.EncodeToString(calculated[:]))
		})
	}
}

func TestObjectRepository_SaveChunk(t *testing.T) {
	t.Parallel()

	t.Run(`Given a disk ObjectRepository,
	when SaveChunk method is called with a hash that does not match the chunk,
	then it returns an error and leaves no files`, func(t *testing.T) {
		t.Parallel()

		data := bytes.Repeat([]byte("chunk "), 4096)
		comp, err := repository.NewCompression(repository.NoneCompressionType, nil)
		require.NoError(t, err)
		repositoryPath := t.TempDir()
		repo, err := disk.NewObjectRepository(repositoryPath)
		require.NoError(t, err)

		err = repo.SaveChunk(context.Background(), writeSourceFile(t, data), strings.Repeat("cd", sha256.Size), 0, 1024, comp, newSymmetricKey(t), 1)

		require.ErrorIs(t, err, object.ErrHashMismatch)
		require.Empty(t, repositoryFiles(t, repositoryPath))
	})

	t.Run(`Given a disk ObjectRepository,
	when SaveChunk method is called with the chunk hash,
	then it stores the chunk`, func(t *testing.T) {
		t.Parallel()

		data := bytes.Repeat([]byte("chunk "), 4096)
		comp, err := repository.NewCompression(repository.NoneCompressionType, nil)
		require.NoError(t, err)
		repositoryPath := t.TempDir()
		repo, err := disk.NewObjectRepository(repositoryPath)
		require.NoError(t, err)
		chunk := data[1024:2048]
		hash := compressedHash(t, comp, chunk)

		err = repo.SaveChunk(context.Background(), writeSourceFile(t, data), hash, 1024, 1024, comp, nil, 0)

		require.NoError(t, err)
		exists, err := repo.AlreadyExists(context.Background(), hash, nil, 0)
		require.NoError(t, err)
		require.True(t, exists)
	})
}

func TestObjectRepository_ListEncryptedObjects(t *testing.T) {
	t.Parallel()

	t.Run(`Given a disk ObjectRepository with an encrypted object and a leftover temporary file,
	when ListEncryptedObjects method is called,
	then it returns only the object hash`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		data := []byte("encrypted object")
		comp, err := repository.NewCompression(repository.NoneCompressionType, nil)
		require.NoError(t, err)
		hash := compressedHash(t, comp, data)
		repositoryPath := t.TempDir()
		repo, err := disk.NewObjectRepository(repositoryPath)
		require.NoError(t, err)
		key := newSymmetricKey(t)
		require.NoError(t, repo.Save(ctx, writeSourceFile(t, data), hash, comp, key, 1))

		leftover := filepath.Join(object.EncryptedGenerationPath(repositoryPath, 1, false), hash[:2], "."+hash[2:]+".tmp-123")
		require.NoError(t, os.WriteFile(leftover, []byte("partial"), 0o600))

		hashes, err := repo.ListEncryptedObjects(ctx, 1)

		require.NoError(t, err)
		require.Equal(t, []string{hash}, hashes)
	})
}

func TestObjectRepository_RestoreObject(t *testing.T) {
	t.Parallel()

	original := bytes.Repeat([]byte("original "), 2048)
	replaced := bytes.Repeat([]byte("replaced "), 2048)

	tests := []struct {
		name        string
		encrypted   bool
		tamper      bool
		expectedErr error
	}{
		{
			name: "with an intact object, then it restores the original content",
		},
		{
			name:      "with an intact encrypted object, then it restores the original content",
			encrypted: true,
		},
		{
			name:        "with an object replaced by another one, then it returns a hash mismatch error and keeps the previous file",
			tamper:      true,
			expectedErr: object.ErrHashMismatch,
		},
		{
			name:        "with an encrypted object replaced by another one, then it returns a hash mismatch error and keeps the previous file",
			encrypted:   true,
			tamper:      true,
			expectedErr: object.ErrHashMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a disk ObjectRepository,
		when RestoreObject method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			comp, err := repository.NewCompression(repository.ZstdCompressionType, nil)
			require.NoError(t, err)
			repositoryPath := t.TempDir()
			repo, err := disk.NewObjectRepository(repositoryPath)
			require.NoError(t, err)

			var key []byte
			if tt.encrypted {
				key = newSymmetricKey(t)
			}
			hash := compressedHash(t, comp, original)
			require.NoError(t, repo.Save(ctx, writeSourceFile(t, original), hash, comp, key, 1))

			if tt.tamper {
				otherHash := compressedHash(t, comp, replaced)
				require.NoError(t, repo.Save(ctx, writeSourceFile(t, replaced), otherHash, comp, key, 1))
				originalPath := object.GetObjectsPath(repositoryPath, hash, 1, key, false).ObjectPath
				otherPath := object.GetObjectsPath(repositoryPath, otherHash, 1, key, false).ObjectPath
				require.NoError(t, os.Rename(otherPath, originalPath))
			}

			destinationDirectory := filepath.Join(t.TempDir(), "restored")
			require.NoError(t, os.MkdirAll(destinationDirectory, 0o755))
			destination := filepath.Join(destinationDirectory, "file.bin")
			previous := []byte("previous content")
			require.NoError(t, os.WriteFile(destination, previous, 0o600))
			size := int64(len(original))
			file := fixtures.FileBuilder{Path: &destination, Size: &size, Hash: &hash}.Build()

			err = repo.RestoreObject(ctx, comp, &file, key, 1)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				kept, readErr := os.ReadFile(destination)
				require.NoError(t, readErr)
				require.Equal(t, previous, kept)
				require.Len(t, repositoryFiles(t, destinationDirectory), 1)
				return
			}
			require.NoError(t, err)
			restored, err := os.ReadFile(destination)
			require.NoError(t, err)
			require.Equal(t, original, restored)
		})
	}
}
