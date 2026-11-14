package blob_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
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

func codecWith(comp *repository.Compression, key []byte, generation uint64) repository.Codec {
	codec := repository.NewCodec(repository.FormatVersionTwo, comp)
	if key == nil {
		return codec
	}
	return codec.WithDataKey(key, generation)
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

			require.NoError(t, repo.Save(ctx, writeSource(t, data), hash, codecWith(comp, key, 3)))

			require.Equal(t, []string{expectedKey}, store.Keys())
			exists, err := repo.AlreadyExists(ctx, hash, codecWith(nil, key, 3))
			require.NoError(t, err)
			require.True(t, exists)
			require.NoError(t, repo.VerifyObject(ctx, hash, codecWith(nil, key, 3)))

			destination := filepath.Join(t.TempDir(), "restored.bin")
			size := int64(len(data))
			file := fixtures.FileBuilder{Path: &destination, Size: &size, Hash: &hash}.Build()
			require.NoError(t, repo.RestoreObject(ctx, &file, codecWith(comp, key, 3)))
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

		require.NoError(t, repo.SaveChunk(ctx, sourcePath, *snapshot.NewChunk(first, 0, half), codecWith(comp, key, 1)))
		require.NoError(t, repo.SaveChunk(ctx, sourcePath, *snapshot.NewChunk(second, half, int64(len(data))-half), codecWith(comp, key, 1)))

		destination := filepath.Join(t.TempDir(), "restored.bin")
		file := snapshot.HydrateFile(destination, "", int64(len(data)), []snapshot.Chunk{
			*snapshot.NewChunk(first, 0, half),
			*snapshot.NewChunk(second, half, int64(len(data))-half),
		})
		require.NoError(t, repo.RestoreObject(ctx, file, codecWith(comp, key, 1)))
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
			err := repo.Save(ctx, writeSource(t, data), hash, codecWith(comp, nil, 0))
			require.ErrorIs(t, err, blob.ErrInvalidHash, hash)
			_, err = repo.AlreadyExists(ctx, hash, codecWith(nil, nil, 0))
			require.ErrorIs(t, err, blob.ErrInvalidHash, hash)
			err = repo.VerifyObject(ctx, hash, codecWith(nil, nil, 0))
			require.ErrorIs(t, err, blob.ErrInvalidHash, hash)
		}
		require.Empty(t, store.Keys())
	})

	t.Run(`Given a blob ObjectRepository,
	when a missing object is verified,
	then it returns a snapshot not found error`, func(t *testing.T) {
		t.Parallel()

		err := blob.NewObjectRepository(blobtest.NewMemoryStore()).VerifyObject(context.Background(), "abcdef", codecWith(nil, nil, 0))

		require.ErrorAs(t, err, new(snapshot.NotFoundError))
	})

	t.Run(`Given a blob ObjectRepository,
	when Save is called with a hash that does not match the source,
	then it returns a hash mismatch error and stores nothing`, func(t *testing.T) {
		t.Parallel()

		store := blobtest.NewMemoryStore()

		err := blob.NewObjectRepository(store).Save(context.Background(), writeSource(t, data), "abcdef", codecWith(zstdCompression(t), nil, 0))

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
		require.NoError(t, repo.Save(ctx, writeSource(t, data), hash, codecWith(comp, nil, 0)))
		other := []byte("other data")
		otherHash := compressedHash(t, comp, other)
		require.NoError(t, repo.Save(ctx, writeSource(t, other), otherHash, codecWith(comp, nil, 0)))
		replacement, err := store.Get(ctx, "objects/"+otherHash[:2]+"/"+otherHash[2:])
		require.NoError(t, err)
		store.Corrupt("objects/"+hash[:2]+"/"+hash[2:], readAll(t, replacement))

		destination := filepath.Join(t.TempDir(), "restored.bin")
		size := int64(len(data))
		file := fixtures.FileBuilder{Path: &destination, Size: &size, Hash: &hash}.Build()
		err = repo.RestoreObject(ctx, &file, codecWith(comp, nil, 0))

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
			require.NoError(t, repo.Save(ctx, writeSource(t, []byte(content)), hash, codecWith(comp, oldKey, 1)))
			hashes = append(hashes, hash)
		}
		require.NoError(t, repo.Save(ctx, writeSource(t, []byte("plain")), compressedHash(t, comp, []byte("plain")), codecWith(comp, nil, 0)))

		listed, err := repo.ListEncryptedObjects(ctx, 1)
		require.NoError(t, err)
		require.ElementsMatch(t, hashes, listed)
		for _, hash := range listed {
			reader, err := repo.ReadEncryptedObject(ctx, hash, codecWith(nil, oldKey, 1))
			require.NoError(t, err)
			require.NoError(t, repo.SaveRekeyedObject(ctx, hash, bytes.NewReader(readAll(t, reader)), codecWith(nil, newKey, 2)))
		}
		require.NoError(t, repo.DeleteEncryptedGeneration(ctx, 1))

		remaining, err := repo.ListEncryptedObjects(ctx, 1)
		require.NoError(t, err)
		require.Empty(t, remaining)
		rekeyed, err := repo.ListEncryptedObjects(ctx, 2)
		require.NoError(t, err)
		require.Equal(t, listed, rekeyed)
		for _, hash := range rekeyed {
			require.NoError(t, repo.VerifyObject(ctx, hash, codecWith(nil, newKey, 2)))
		}
		require.Len(t, store.Keys(), 4)
	})

	t.Run(`Given a blob ObjectRepository,
	when SaveRekeyedObject is called with data that does not match the hash,
	then it returns a hash mismatch error and stores nothing`, func(t *testing.T) {
		t.Parallel()

		store := blobtest.NewMemoryStore()

		err := blob.NewObjectRepository(store).SaveRekeyedObject(context.Background(), "abcdef", bytes.NewReader([]byte("tampered")), codecWith(nil, newKey(t), 2))

		require.ErrorIs(t, err, object.ErrHashMismatch)
		require.Empty(t, store.Keys())
	})

	t.Run(`Given a blob ObjectRepository,
	when the rekey methods are called without a symmetric key or a source,
	then they return the corresponding errors`, func(t *testing.T) {
		t.Parallel()

		repo := blob.NewObjectRepository(blobtest.NewMemoryStore())

		_, err := repo.ReadEncryptedObject(context.Background(), "abcdef", codecWith(nil, nil, 1))
		require.ErrorIs(t, err, blob.ErrSymmetricKeyRequired)
		err = repo.SaveRekeyedObject(context.Background(), "abcdef", bytes.NewReader(nil), codecWith(nil, nil, 1))
		require.ErrorIs(t, err, blob.ErrSymmetricKeyRequired)
		err = repo.SaveRekeyedObject(context.Background(), "abcdef", nil, codecWith(nil, newKey(t), 1))
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
		require.NoError(t, repo.Save(ctx, writeSource(t, first), firstHash, codecWith(comp, nil, 0)))
		require.NoError(t, repo.Save(ctx, writeSource(t, second), secondHash, codecWith(comp, nil, 0)))
		require.NoError(t, repo.Save(ctx, writeSource(t, first), firstHash, codecWith(comp, key, 2)))
		require.NoError(t, store.Put(ctx, "objects/readme.txt", func(io.Writer) error { return nil }))

		plain, err := repo.ListObjects(ctx, false, 0)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{firstHash, secondHash}, snapshot.StoredHashes(plain))
		for _, object := range plain {
			require.Positive(t, object.Size())
		}
		encrypted, err := repo.ListObjects(ctx, true, 2)
		require.NoError(t, err)
		require.Equal(t, []string{firstHash}, snapshot.StoredHashes(encrypted))
		plainFirst := plain[slices.IndexFunc(plain, func(object snapshot.StoredObject) bool { return object.Hash() == firstHash })]
		require.Greater(t, encrypted[0].Size(), plainFirst.Size(), "the encrypted object carries a nonce and a tag")

		require.NoError(t, repo.DeleteObject(ctx, secondHash, false, 0))
		require.NoError(t, repo.DeleteObject(ctx, firstHash, true, 2))

		plain, err = repo.ListObjects(ctx, false, 0)
		require.NoError(t, err)
		require.Equal(t, []string{firstHash}, snapshot.StoredHashes(plain))
		encrypted, err = repo.ListObjects(ctx, true, 2)
		require.NoError(t, err)
		require.Empty(t, encrypted)
		require.ErrorIs(t, repo.DeleteObject(ctx, "../x", false, 0), blob.ErrInvalidHash)
	})
}

func TestObjectRepository_ListEncryptedGenerations(t *testing.T) {
	t.Parallel()

	t.Run(`Given a blob ObjectRepository with objects in several encrypted generations and plain objects,
	when ListEncryptedGenerations is called,
	then it returns each generation once, sorted`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		store := blobtest.NewMemoryStore()
		repo := blob.NewObjectRepository(store)
		comp := zstdCompression(t)
		key := newKey(t)
		for i, generation := range []uint64{3, 1, 3} {
			data := []byte{byte(i)}
			require.NoError(t, repo.Save(ctx, writeSource(t, data), compressedHash(t, comp, data), codecWith(comp, key, generation)))
		}
		plain := []byte("plain")
		require.NoError(t, repo.Save(ctx, writeSource(t, plain), compressedHash(t, comp, plain), codecWith(comp, nil, 0)))
		require.NoError(t, store.Put(ctx, "objects/encrypted/notes.txt", func(io.Writer) error { return nil }))
		require.NoError(t, store.Put(ctx, "objects/encrypted/x/ab/cd", func(io.Writer) error { return nil }))

		generations, err := repo.ListEncryptedGenerations(ctx)

		require.NoError(t, err)
		require.Equal(t, []uint64{1, 3}, generations)
	})
}

func TestObjectRepository_VerifyObject_Tampered(t *testing.T) {
	t.Parallel()

	t.Run(`Given a blob ObjectRepository with an encrypted object whose bytes were replaced,
	when VerifyObject is called,
	then it returns a corrupt object error`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		store := blobtest.NewMemoryStore()
		repo := blob.NewObjectRepository(store)
		comp := zstdCompression(t)
		data := []byte("object that will be tampered")
		hash := compressedHash(t, comp, data)
		key := newKey(t)
		require.NoError(t, repo.Save(ctx, writeSource(t, data), hash, codecWith(comp, key, 1)))
		store.Corrupt("objects/encrypted/1/"+hash[:2]+"/"+hash[2:], bytes.Repeat([]byte("x"), 64))

		err := repo.VerifyObject(ctx, hash, codecWith(nil, key, 1))

		require.ErrorIs(t, err, snapshot.ErrCorruptObject)
	})
}

type cleaningStore struct {
	*blobtest.MemoryStore
	prefixes []string
	removed  int
	err      error
}

func (c *cleaningStore) DeleteTemporaries(_ context.Context, prefix string) (int, error) {
	c.prefixes = append(c.prefixes, prefix)
	return c.removed, c.err
}

func TestObjectRepository_DeleteTemporaryObjects(t *testing.T) {
	t.Parallel()

	errStore := errors.New("store failed")
	tests := []struct {
		name     string
		store    blob.Store
		expected int
		err      error
	}{
		{name: "on a store without temporary files, then it removes nothing", store: blobtest.NewMemoryStore()},
		{name: "on a store that cleans temporary files, then it returns how many it removed", store: &cleaningStore{MemoryStore: blobtest.NewMemoryStore(), removed: 3}, expected: 3},
		{name: "on a store that fails to clean temporary files, then it returns the error", store: &cleaningStore{MemoryStore: blobtest.NewMemoryStore(), err: errStore}, err: errStore},
	}
	for _, tt := range tests {
		t.Run(`Given a blob ObjectRepository,
		when DeleteTemporaryObjects is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			removed, err := blob.NewObjectRepository(tt.store).DeleteTemporaryObjects(context.Background())

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, removed)
			if cleaner, ok := tt.store.(*cleaningStore); ok {
				require.Equal(t, []string{"objects"}, cleaner.prefixes)
			}
		})
	}
}

func v3Codec(comp *repository.Compression, dataKey, metadataKey []byte, generation uint64) repository.Codec {
	codec := repository.NewCodec(repository.FormatVersionThree, comp)
	if dataKey == nil {
		return codec
	}
	return codec.WithDataKey(dataKey, generation).WithMetadataKey(metadataKey)
}

func contentID(t *testing.T, codec repository.Codec, data []byte) string {
	t.Helper()
	hash, err := object.Digest(context.Background(), bytes.NewReader(data), codec, compression.NewHandlersFactory())
	require.NoError(t, err)
	return hash
}

func TestObjectRepository_ContentAddressed(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte("stowmark v3 blob "), 8192)
	metadataKey := newKey(t)

	for _, encrypted := range []bool{false, true} {
		t.Run(`Given a format version 3 blob ObjectRepository,
		when an object is saved, verified and restored,
		then it is stored under its content id and restores the original data`, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			store := blobtest.NewMemoryStore()
			repo := blob.NewObjectRepository(store)
			var key []byte
			if encrypted {
				key = newKey(t)
			}
			codec := v3Codec(zstdCompression(t), key, metadataKey, 4)
			hash := contentID(t, codec, data)
			expectedKey := "objects/" + hash[:2] + "/" + hash[2:]
			if encrypted {
				expectedKey = "objects/encrypted/4/" + hash[:2] + "/" + hash[2:]
			}

			require.NoError(t, repo.Save(ctx, writeSource(t, data), hash, codec))

			require.Equal(t, []string{expectedKey}, store.Keys())
			require.NoError(t, repo.VerifyObject(ctx, hash, codec))
			destination := filepath.Join(t.TempDir(), "restored.bin")
			file := *snapshot.HydrateFile(destination, hash, int64(len(data)), nil)
			require.NoError(t, repo.RestoreObject(ctx, &file, codec.WithCompression(nil)))
			restored, err := os.ReadFile(destination)
			require.NoError(t, err)
			require.Equal(t, data, restored)
		})
	}

	t.Run(`Given an encrypted format version 3 blob ObjectRepository with two objects,
	when one object is copied over the other,
	then VerifyObject reports it as corrupt because the id is bound to the encryption`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		store := blobtest.NewMemoryStore()
		repo := blob.NewObjectRepository(store)
		codec := v3Codec(zstdCompression(t), newKey(t), metadataKey, 1)
		first := contentID(t, codec, []byte("first"))
		second := contentID(t, codec, []byte("second"))
		require.NoError(t, repo.Save(ctx, writeSource(t, []byte("first")), first, codec))
		require.NoError(t, repo.Save(ctx, writeSource(t, []byte("second")), second, codec))
		replacement, err := store.Get(ctx, "objects/encrypted/1/"+second[:2]+"/"+second[2:])
		require.NoError(t, err)
		store.Corrupt("objects/encrypted/1/"+first[:2]+"/"+first[2:], readAll(t, replacement))

		err = repo.VerifyObject(ctx, first, codec)

		require.ErrorIs(t, err, snapshot.ErrCorruptObject)
		require.NoError(t, repo.VerifyObject(ctx, second, codec))
	})

	t.Run(`Given an encrypted format version 3 object in generation 1,
	when it is read with the old key and saved into generation 2 with a new key,
	then the new generation keeps the same id and verifies only with the new key`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repo := blob.NewObjectRepository(blobtest.NewMemoryStore())
		oldCodec := v3Codec(zstdCompression(t), newKey(t), metadataKey, 1)
		newCodec := v3Codec(zstdCompression(t), newKey(t), metadataKey, 2)
		hash := contentID(t, oldCodec, data)
		require.Equal(t, hash, contentID(t, newCodec, data), "the id does not depend on the data key")
		require.NoError(t, repo.Save(ctx, writeSource(t, data), hash, oldCodec))

		reader, err := repo.ReadEncryptedObject(ctx, hash, oldCodec)
		require.NoError(t, err)
		require.NoError(t, repo.SaveRekeyedObject(ctx, hash, bytes.NewReader(readAll(t, reader)), newCodec))

		require.NoError(t, repo.VerifyObject(ctx, hash, newCodec))
		require.ErrorIs(t, repo.VerifyObject(ctx, hash, newCodec.WithDataKey(oldCodec.DataKey(), 2)), snapshot.ErrCorruptObject)
	})

	t.Run(`Given an encrypted format version 3 object,
	when it is read for a rekey with another key,
	then it returns a corrupt object error`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repo := blob.NewObjectRepository(blobtest.NewMemoryStore())
		codec := v3Codec(zstdCompression(t), newKey(t), metadataKey, 1)
		hash := contentID(t, codec, data)
		require.NoError(t, repo.Save(ctx, writeSource(t, data), hash, codec))

		reader, err := repo.ReadEncryptedObject(ctx, hash, codec.WithDataKey(newKey(t), 1))
		if err == nil {
			_, err = io.ReadAll(reader)
		}

		require.ErrorIs(t, err, encrypt.ErrDecryptionFailed)
	})
}
