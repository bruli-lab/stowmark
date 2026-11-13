package blob_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/bruli-lab/stowmark/internal/infra/blob/blobtest"
	"github.com/stretchr/testify/require"
)

func manifestAt(t *testing.T, id string, createdAt time.Time) *snapshot.Manifest {
	t.Helper()

	comp := zstdCompression(t)
	file := snapshot.File{}
	file.Hydrate("/source/file.txt", "abcdef", 3, []snapshot.Chunk{})
	manifest := fixtures.ManifestBuilder{ID: &id, CreatedAt: &createdAt, Files: []snapshot.File{file}, Compression: comp}.Build()

	return &manifest
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
		require.NoError(t, repo.Save(ctx, older))
		require.NoError(t, repo.Save(ctx, newer))
		require.NoError(t, store.Put(ctx, "snapshots/notes.txt", func(w io.Writer) error {
			_, err := io.WriteString(w, "not a manifest")
			return err
		}))

		got, err := repo.Get(ctx, older.Id())
		require.NoError(t, err)
		require.Equal(t, older.Id(), got.Id())
		require.Equal(t, older.Files(), got.Files())

		listed, err := repo.List(ctx)
		require.NoError(t, err)
		require.Len(t, listed, 2)
		require.Equal(t, newer.Id(), listed[0].Id())
		require.Equal(t, older.Id(), listed[1].Id())
	})

	t.Run(`Given an empty blob ManifestRepository,
	when List is called,
	then it returns no manifests`, func(t *testing.T) {
		t.Parallel()

		listed, err := blob.NewManifestRepository(blobtest.NewMemoryStore()).List(context.Background())

		require.NoError(t, err)
		require.Empty(t, listed)
	})

	t.Run(`Given an empty blob ManifestRepository,
	when Get is called with a missing snapshot id,
	then it returns a repository not found error`, func(t *testing.T) {
		t.Parallel()

		_, err := blob.NewManifestRepository(blobtest.NewMemoryStore()).Get(context.Background(), "20261113T000000Z-missing0")

		require.ErrorAs(t, err, new(*repository.NotFoundError))
	})

	t.Run(`Given a blob ManifestRepository,
	when Get is called with a snapshot id that escapes the snapshots folder,
	then it returns an invalid key error`, func(t *testing.T) {
		t.Parallel()

		repo := blob.NewManifestRepository(blobtest.NewMemoryStore())

		for _, id := range []string{"../config", "a/b", ".hidden", ""} {
			_, err := repo.Get(context.Background(), id)
			require.ErrorIs(t, err, blob.ErrInvalidKey, id)
		}
	})

	t.Run(`Given a blob ManifestRepository with a corrupted manifest,
	when List is called,
	then it returns an error`, func(t *testing.T) {
		t.Parallel()

		store := blobtest.NewMemoryStore()
		store.Corrupt("snapshots/20261113T000000Z-cccccccc.json", []byte("{"))

		_, err := blob.NewManifestRepository(store).List(context.Background())

		require.Error(t, err)
	})
}
