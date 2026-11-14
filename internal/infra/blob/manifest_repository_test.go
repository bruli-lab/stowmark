package blob_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/bruli-lab/stowmark/internal/infra/blob/blobtest"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/stretchr/testify/require"
)

func manifestAt(t *testing.T, id string, createdAt time.Time) *snapshot.Manifest {
	t.Helper()

	comp := zstdCompression(t)
	file := *snapshot.HydrateFile("/source/file.txt", "abcdef", 3, []snapshot.Chunk{})
	manifest := fixtures.ManifestBuilder{ID: &id, CreatedAt: &createdAt, Files: []snapshot.File{file}, Compression: comp}.Build()

	return &manifest
}

var plainCodec = repository.NewCodec(repository.FormatVersionTwo, nil)

func encryptedCodec(metadataByte byte) repository.Codec {
	return repository.NewCodec(repository.FormatVersionThree, nil).
		WithDataKey(bytes.Repeat([]byte{7}, encrypt.AES256KeySize), 1).
		WithMetadataKey(bytes.Repeat([]byte{metadataByte}, encrypt.AES256KeySize))
}

func TestManifestRepository(t *testing.T) {
	t.Parallel()

	t.Run(`Given a blob ManifestRepository with two manifests and an unrelated blob,
	when Get and List are called,
	then they return the manifests, newest first`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		store := blobtest.NewMemoryStore()
		repo := blob.NewManifestRepository(store)
		older := manifestAt(t, "20261101T000000Z-aaaaaaaa", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
		newer := manifestAt(t, "20261113T000000Z-bbbbbbbb", time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC))
		require.NoError(t, repo.Save(ctx, older, plainCodec))
		require.NoError(t, repo.Save(ctx, newer, plainCodec))
		require.NoError(t, store.Put(ctx, "snapshots/notes.txt", func(w io.Writer) error {
			_, err := io.WriteString(w, "not a manifest")
			return err
		}))

		got, err := repo.Get(ctx, older.ID(), plainCodec)
		require.NoError(t, err)
		require.Equal(t, older.ID(), got.ID())
		require.Equal(t, older.Files(), got.Files())

		listed, err := repo.List(ctx, plainCodec)
		require.NoError(t, err)
		require.Len(t, listed, 2)
		require.Equal(t, newer.ID(), listed[0].ID())
		require.Equal(t, older.ID(), listed[1].ID())
	})

	t.Run(`Given an empty blob ManifestRepository,
	when List is called,
	then it returns no manifests`, func(t *testing.T) {
		t.Parallel()

		listed, err := blob.NewManifestRepository(blobtest.NewMemoryStore()).List(context.Background(), plainCodec)

		require.NoError(t, err)
		require.Empty(t, listed)
	})

	t.Run(`Given an empty blob ManifestRepository,
	when Get is called with a missing snapshot id,
	then it returns a repository not found error`, func(t *testing.T) {
		t.Parallel()

		_, err := blob.NewManifestRepository(blobtest.NewMemoryStore()).Get(context.Background(), "20261113T000000Z-missing0", plainCodec)

		require.ErrorAs(t, err, new(*repository.NotFoundError))
	})

	t.Run(`Given a blob ManifestRepository,
	when Get is called with a snapshot id that escapes the snapshots folder,
	then it returns an invalid key error`, func(t *testing.T) {
		t.Parallel()

		repo := blob.NewManifestRepository(blobtest.NewMemoryStore())

		for _, id := range []string{"../config", "a/b", ".hidden", ""} {
			_, err := repo.Get(context.Background(), id, plainCodec)
			require.ErrorIs(t, err, blob.ErrInvalidKey, id)
		}
	})

	t.Run(`Given a blob ManifestRepository with two manifests,
	when Delete is called with one id and then with a missing id,
	then only the other manifest is listed and the missing id does not fail`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repo := blob.NewManifestRepository(blobtest.NewMemoryStore())
		kept := manifestAt(t, "20261101T000000Z-aaaaaaaa", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
		deleted := manifestAt(t, "20261113T000000Z-bbbbbbbb", time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC))
		require.NoError(t, repo.Save(ctx, kept, plainCodec))
		require.NoError(t, repo.Save(ctx, deleted, plainCodec))

		require.NoError(t, repo.Delete(ctx, deleted.ID()))
		require.NoError(t, repo.Delete(ctx, deleted.ID()))

		listed, err := repo.List(ctx, plainCodec)
		require.NoError(t, err)
		require.Len(t, listed, 1)
		require.Equal(t, kept.ID(), listed[0].ID())
		require.ErrorIs(t, repo.Delete(ctx, "../config"), blob.ErrInvalidKey)
	})

	t.Run(`Given a blob ManifestRepository with a valid manifest, a corrupted one and unrelated blobs,
	when ListIDs is called,
	then it returns the sorted ids of every manifest without reading them`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		store := blobtest.NewMemoryStore()
		repo := blob.NewManifestRepository(store)
		require.NoError(t, repo.Save(ctx, manifestAt(t, "20261113T000000Z-bbbbbbbb", time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)), plainCodec))
		store.Corrupt("snapshots/20261101T000000Z-aaaaaaaa.json", []byte("{"))
		require.NoError(t, store.Put(ctx, "snapshots/notes.txt", func(io.Writer) error { return nil }))

		ids, err := repo.ListIDs(ctx)

		require.NoError(t, err)
		require.Equal(t, []string{"20261101T000000Z-aaaaaaaa", "20261113T000000Z-bbbbbbbb"}, ids)
	})

	t.Run(`Given a blob ManifestRepository with a corrupted manifest,
	when List is called,
	then it returns an error`, func(t *testing.T) {
		t.Parallel()

		store := blobtest.NewMemoryStore()
		store.Corrupt("snapshots/20261113T000000Z-cccccccc.json", []byte("{"))

		_, err := blob.NewManifestRepository(store).List(context.Background(), plainCodec)

		require.Error(t, err)
	})
}

func TestManifestRepository_Encrypted(t *testing.T) {
	t.Parallel()

	id := "20261114T000000Z-cccccccc"
	createdAt := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)

	t.Run(`Given an encrypted format version 3 repository,
	when a manifest is saved and read back,
	then it is stored as an encryption stream without readable paths and returns the same manifest`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		store := blobtest.NewMemoryStore()
		repo := blob.NewManifestRepository(store)
		manifest := manifestAt(t, id, createdAt)

		require.NoError(t, repo.Save(ctx, manifest, encryptedCodec(1)))

		stored := readBlob(t, store, "snapshots/"+id+".json")
		require.True(t, encrypt.IsStream(stored))
		require.NotContains(t, string(stored), "/source/file.txt")
		got, err := repo.Get(ctx, id, encryptedCodec(1))
		require.NoError(t, err)
		require.Equal(t, manifest.Files(), got.Files())
		listed, err := repo.List(ctx, encryptedCodec(1))
		require.NoError(t, err)
		require.Len(t, listed, 1)
	})

	tests := []struct {
		name        string
		prepare     func(t *testing.T, store *blobtest.MemoryStore)
		codec       repository.Codec
		expectedErr error
	}{
		{
			name: "without the metadata key, then it returns an encrypted manifest error",
			prepare: func(t *testing.T, store *blobtest.MemoryStore) {
				require.NoError(t, blob.NewManifestRepository(store).Save(context.Background(), manifestAt(t, id, createdAt), encryptedCodec(1)))
			},
			codec:       plainCodec,
			expectedErr: blob.ErrEncryptedManifest,
		},
		{
			name: "with another metadata key, then it fails to decrypt",
			prepare: func(t *testing.T, store *blobtest.MemoryStore) {
				require.NoError(t, blob.NewManifestRepository(store).Save(context.Background(), manifestAt(t, id, createdAt), encryptedCodec(1)))
			},
			codec:       encryptedCodec(2),
			expectedErr: encrypt.ErrDecryptionFailed,
		},
		{
			name: "with a manifest of another snapshot copied under this id, then it fails to decrypt because the id is the associated data",
			prepare: func(t *testing.T, store *blobtest.MemoryStore) {
				other := "20261114T000000Z-dddddddd"
				require.NoError(t, blob.NewManifestRepository(store).Save(context.Background(), manifestAt(t, other, createdAt), encryptedCodec(1)))
				store.Corrupt("snapshots/"+id+".json", readBlob(t, store, "snapshots/"+other+".json"))
			},
			codec:       encryptedCodec(1),
			expectedErr: encrypt.ErrDecryptionFailed,
		},
		{
			name: "with a tampered manifest, then it fails to decrypt",
			prepare: func(t *testing.T, store *blobtest.MemoryStore) {
				require.NoError(t, blob.NewManifestRepository(store).Save(context.Background(), manifestAt(t, id, createdAt), encryptedCodec(1)))
				stored := readBlob(t, store, "snapshots/"+id+".json")
				stored[len(stored)-5] ^= 1
				store.Corrupt("snapshots/"+id+".json", stored)
			},
			codec:       encryptedCodec(1),
			expectedErr: encrypt.ErrDecryptionFailed,
		},
		{
			name: "with a manifest in clear, then it returns an unencrypted manifest error so nobody can replace it",
			prepare: func(t *testing.T, store *blobtest.MemoryStore) {
				require.NoError(t, blob.NewManifestRepository(store).Save(context.Background(), manifestAt(t, id, createdAt), plainCodec))
			},
			codec:       encryptedCodec(1),
			expectedErr: blob.ErrUnencryptedManifest,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a format version 3 manifest,
		when Get is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			store := blobtest.NewMemoryStore()
			tt.prepare(t, store)

			_, err := blob.NewManifestRepository(store).Get(context.Background(), id, tt.codec)

			require.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func readBlob(t *testing.T, store *blobtest.MemoryStore, key string) []byte {
	t.Helper()
	reader, err := store.Get(context.Background(), key)
	require.NoError(t, err)
	defer func() {
		_ = reader.Close()
	}()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	return data
}
