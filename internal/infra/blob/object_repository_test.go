package blob_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/bruli-lab/stowmark/internal/infra/blob/blobtest"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/object"
	"github.com/stretchr/testify/require"
)

func newKey(t *testing.T) []byte {
	t.Helper()

	key := make([]byte, encrypt.AES256KeySize)
	_, err := rand.Read(key)
	require.NoError(t, err)

	return key
}

func zstdCompression(t *testing.T) *repository.Compression {
	t.Helper()

	comp, err := repository.NewCompression(repository.ZstdCompressionType, nil)
	require.NoError(t, err)

	return comp
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

func writeSource(t *testing.T, data []byte) string {
	t.Helper()

	filePath := filepath.Join(t.TempDir(), "source.bin")
	require.NoError(t, os.WriteFile(filePath, data, 0o600))

	return filePath
}

func readAll(t *testing.T, reader io.ReadCloser) []byte {
	t.Helper()

	defer func() {
		_ = reader.Close()
	}()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)

	return data
}

func TestObjectRepository_SaveAndRestore(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte("stowmark blob "), 4096)

	for _, encrypted := range []bool{false, true} {
		t.Run(`Given a blob ObjectRepository,
		when an object is saved and restored,
		then it exists under its content-addressed key and restores the original data`, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			store := blobtest.NewMemoryStore()
			repo := blob.NewObjectRepository(store)
			comp := zstdCompression(t)
			hash := compressedHash(t, comp, data)
			var key []byte
			expectedKey := "objects/" + hash[:2] + "/" + hash[2:]
			if encrypted {
				key = newKey(t)
				expectedKey = "objects/encrypted/3/" + hash[:2] + "/" + hash[2:]
			}

			require.NoError(t, repo.Save(ctx, writeSource(t, data), hash, comp, key, 3))

			require.Equal(t, []string{expectedKey}, store.Keys())
			exists, err := repo.AlreadyExists(ctx, hash, key, 3)
			require.NoError(t, err)
			require.True(t, exists)
			reader, err := repo.ReadObject(ctx, hash, key, 3)
			require.NoError(t, err)
			stored := sha256.Sum256(readAll(t, reader))
			require.Equal(t, hash, hex.EncodeToString(stored[:]))

			destination := filepath.Join(t.TempDir(), "restored.bin")
			size := int64(len(data))
			file := fixtures.FileBuilder{Path: &destination, Size: &size, Hash: &hash}.Build()
			require.NoError(t, repo.RestoreObject(ctx, comp, &file, key, 3))
			restored, err := os.ReadFile(destination)
			require.NoError(t, err)
			require.Equal(t, data, restored)
		})
	}
}

func TestObjectRepository_SaveChunk(t *testing.T) {
	t.Parallel()

	t.Run(`Given a blob ObjectRepository and a file split in two chunks,
	when both chunks are saved and the file is restored,
	then it restores the original data`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repo := blob.NewObjectRepository(blobtest.NewMemoryStore())
		comp := zstdCompression(t)
		key := newKey(t)
		data := bytes.Repeat([]byte("0123456789"), 2048)
		sourcePath := writeSource(t, data)
		half := int64(len(data) / 2)
		first := compressedHash(t, comp, data[:half])
		second := compressedHash(t, comp, data[half:])

		require.NoError(t, repo.SaveChunk(ctx, sourcePath, first, 0, half, comp, key, 1))
		require.NoError(t, repo.SaveChunk(ctx, sourcePath, second, half, int64(len(data))-half, comp, key, 1))

		destination := filepath.Join(t.TempDir(), "restored.bin")
		file := snapshot.File{}
		file.Hydrate(destination, "", int64(len(data)), []snapshot.Chunk{
			*snapshot.NewChunk(first, 0, half),
			*snapshot.NewChunk(second, half, int64(len(data))-half),
		})
		require.NoError(t, repo.RestoreObject(ctx, comp, &file, key, 1))
		restored, err := os.ReadFile(destination)
		require.NoError(t, err)
		require.Equal(t, data, restored)
	})
}

func TestObjectRepository_Errors(t *testing.T) {
	t.Parallel()

	data := []byte("object data")

	t.Run(`Given a blob ObjectRepository,
	when it is called with hashes that are not lowercase hexadecimal,
	then it returns an invalid hash error and never reaches the store`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		store := blobtest.NewMemoryStore()
		repo := blob.NewObjectRepository(store)
		comp := zstdCompression(t)

		for _, hash := range []string{"", "ab", "../../../etc/passwd", "AB12", "ab/cd", "ab-cd"} {
			err := repo.Save(ctx, writeSource(t, data), hash, comp, nil, 0)
			require.ErrorIs(t, err, blob.ErrInvalidHash, hash)
			_, err = repo.AlreadyExists(ctx, hash, nil, 0)
			require.ErrorIs(t, err, blob.ErrInvalidHash, hash)
			_, err = repo.ReadObject(ctx, hash, nil, 0)
			require.ErrorIs(t, err, blob.ErrInvalidHash, hash)
		}
		require.Empty(t, store.Keys())
	})

	t.Run(`Given a blob ObjectRepository,
	when a missing object is read,
	then it returns a snapshot not found error`, func(t *testing.T) {
		t.Parallel()

		_, err := blob.NewObjectRepository(blobtest.NewMemoryStore()).ReadObject(context.Background(), "abcdef", nil, 0)

		require.ErrorAs(t, err, new(snapshot.NotFoundError))
	})

	t.Run(`Given a blob ObjectRepository,
	when Save is called with a hash that does not match the source,
	then it returns a hash mismatch error and stores nothing`, func(t *testing.T) {
		t.Parallel()

		store := blobtest.NewMemoryStore()

		err := blob.NewObjectRepository(store).Save(context.Background(), writeSource(t, data), "abcdef", zstdCompression(t), nil, 0)

		require.ErrorIs(t, err, object.ErrHashMismatch)
		require.Empty(t, store.Keys())
	})

	t.Run(`Given a blob ObjectRepository with an object replaced in the store,
	when the object is restored,
	then it returns a hash mismatch error`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		store := blobtest.NewMemoryStore()
		repo := blob.NewObjectRepository(store)
		comp := zstdCompression(t)
		hash := compressedHash(t, comp, data)
		require.NoError(t, repo.Save(ctx, writeSource(t, data), hash, comp, nil, 0))
		other := []byte("other data")
		otherHash := compressedHash(t, comp, other)
		require.NoError(t, repo.Save(ctx, writeSource(t, other), otherHash, comp, nil, 0))
		replacement, err := store.Get(ctx, "objects/"+otherHash[:2]+"/"+otherHash[2:])
		require.NoError(t, err)
		store.Corrupt("objects/"+hash[:2]+"/"+hash[2:], readAll(t, replacement))

		destination := filepath.Join(t.TempDir(), "restored.bin")
		size := int64(len(data))
		file := fixtures.FileBuilder{Path: &destination, Size: &size, Hash: &hash}.Build()
		err = repo.RestoreObject(ctx, comp, &file, nil, 0)

		require.ErrorIs(t, err, object.ErrHashMismatch)
		require.NoFileExists(t, destination)
	})
}

func TestObjectRepository_Rekey(t *testing.T) {
	t.Parallel()

	t.Run(`Given a blob ObjectRepository with objects in generation 1,
	when they are listed, read, saved into generation 2 with a new key and generation 1 is deleted,
	then generation 2 holds the same objects readable with the new key`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		store := blobtest.NewMemoryStore()
		repo := blob.NewObjectRepository(store)
		comp := zstdCompression(t)
		oldKey := newKey(t)
		newKey := newKey(t)
		var hashes []string
		for _, content := range []string{"first", "second", "third"} {
			hash := compressedHash(t, comp, []byte(content))
			require.NoError(t, repo.Save(ctx, writeSource(t, []byte(content)), hash, comp, oldKey, 1))
			hashes = append(hashes, hash)
		}
		require.NoError(t, repo.Save(ctx, writeSource(t, []byte("plain")), compressedHash(t, comp, []byte("plain")), comp, nil, 0))

		listed, err := repo.ListEncryptedObjects(ctx, 1)
		require.NoError(t, err)
		require.ElementsMatch(t, hashes, listed)
		for _, hash := range listed {
			reader, err := repo.ReadEncryptedObject(ctx, hash, 1, oldKey)
			require.NoError(t, err)
			require.NoError(t, repo.SaveRekeyedObject(ctx, hash, bytes.NewReader(readAll(t, reader)), 2, newKey))
		}
		require.NoError(t, repo.DeleteEncryptedGeneration(ctx, 1))

		remaining, err := repo.ListEncryptedObjects(ctx, 1)
		require.NoError(t, err)
		require.Empty(t, remaining)
		rekeyed, err := repo.ListEncryptedObjects(ctx, 2)
		require.NoError(t, err)
		require.Equal(t, listed, rekeyed)
		for _, hash := range rekeyed {
			reader, err := repo.ReadObject(ctx, hash, newKey, 2)
			require.NoError(t, err)
			stored := sha256.Sum256(readAll(t, reader))
			require.Equal(t, hash, hex.EncodeToString(stored[:]))
		}
		require.Len(t, store.Keys(), 4)
	})

	t.Run(`Given a blob ObjectRepository,
	when SaveRekeyedObject is called with data that does not match the hash,
	then it returns a hash mismatch error and stores nothing`, func(t *testing.T) {
		t.Parallel()

		store := blobtest.NewMemoryStore()

		err := blob.NewObjectRepository(store).SaveRekeyedObject(context.Background(), "abcdef", bytes.NewReader([]byte("tampered")), 2, newKey(t))

		require.ErrorIs(t, err, object.ErrHashMismatch)
		require.Empty(t, store.Keys())
	})

	t.Run(`Given a blob ObjectRepository,
	when the rekey methods are called without a symmetric key or a source,
	then they return the corresponding errors`, func(t *testing.T) {
		t.Parallel()

		repo := blob.NewObjectRepository(blobtest.NewMemoryStore())

		_, err := repo.ReadEncryptedObject(context.Background(), "abcdef", 1, nil)
		require.ErrorIs(t, err, blob.ErrSymmetricKeyRequired)
		err = repo.SaveRekeyedObject(context.Background(), "abcdef", bytes.NewReader(nil), 1, nil)
		require.ErrorIs(t, err, blob.ErrSymmetricKeyRequired)
		err = repo.SaveRekeyedObject(context.Background(), "abcdef", nil, 1, newKey(t))
		require.ErrorIs(t, err, blob.ErrSourceRequired)
	})
}

func TestObjectRepository_ListAndDeleteObjects(t *testing.T) {
	t.Parallel()

	t.Run(`Given a blob ObjectRepository with plain objects, encrypted objects and unrelated blobs,
	when ListObjects and DeleteObject are called,
	then each namespace lists only its own objects and a deleted object disappears`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		store := blobtest.NewMemoryStore()
		repo := blob.NewObjectRepository(store)
		comp := zstdCompression(t)
		first := []byte("first object")
		second := []byte("second object")
		firstHash := compressedHash(t, comp, first)
		secondHash := compressedHash(t, comp, second)
		key := newKey(t)
		require.NoError(t, repo.Save(ctx, writeSource(t, first), firstHash, comp, nil, 0))
		require.NoError(t, repo.Save(ctx, writeSource(t, second), secondHash, comp, nil, 0))
		require.NoError(t, repo.Save(ctx, writeSource(t, first), firstHash, comp, key, 2))
		require.NoError(t, store.Put(ctx, "objects/readme.txt", func(io.Writer) error { return nil }))

		plain, err := repo.ListObjects(ctx, false, 0)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{firstHash, secondHash}, plain)
		encrypted, err := repo.ListObjects(ctx, true, 2)
		require.NoError(t, err)
		require.Equal(t, []string{firstHash}, encrypted)

		require.NoError(t, repo.DeleteObject(ctx, secondHash, false, 0))
		require.NoError(t, repo.DeleteObject(ctx, firstHash, true, 2))

		plain, err = repo.ListObjects(ctx, false, 0)
		require.NoError(t, err)
		require.Equal(t, []string{firstHash}, plain)
		encrypted, err = repo.ListObjects(ctx, true, 2)
		require.NoError(t, err)
		require.Empty(t, encrypted)
		require.ErrorIs(t, repo.DeleteObject(ctx, "../x", false, 0), blob.ErrInvalidHash)
	})
}
