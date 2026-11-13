package blobtest

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/stretchr/testify/require"
)

var errWriteFailed = errors.New("write failed")

func putString(t *testing.T, store blob.Store, key, content string) {
	t.Helper()

	err := store.Put(context.Background(), key, func(destination io.Writer) error {
		_, err := io.WriteString(destination, content)
		return err
	})
	require.NoError(t, err)
}

func getString(t *testing.T, store blob.Store, key string) string {
	t.Helper()

	reader, err := store.Get(context.Background(), key)
	require.NoError(t, err)
	defer func() {
		_ = reader.Close()
	}()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)

	return string(data)
}

func RunStoreContract(t *testing.T, newStore func(t *testing.T) blob.Store) {
	t.Helper()

	t.Run(`Given an empty store,
	when a missing key is read,
	then Get returns a not found error and Exists returns false`, func(t *testing.T) {
		store := newStore(t)

		_, err := store.Get(context.Background(), "objects/ab/missing")
		require.ErrorIs(t, err, blob.ErrNotFound)

		exists, err := store.Exists(context.Background(), "objects/ab/missing")
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run(`Given a store,
	when a blob is put,
	then it can be read back and it exists`, func(t *testing.T) {
		store := newStore(t)

		putString(t, store, "objects/ab/cdef", "content")

		require.Equal(t, "content", getString(t, store, "objects/ab/cdef"))
		exists, err := store.Exists(context.Background(), "objects/ab/cdef")
		require.NoError(t, err)
		require.True(t, exists)
	})

	t.Run(`Given a store,
	when Put is called with a write function that fails after writing data,
	then it returns the error and leaves nothing visible`, func(t *testing.T) {
		store := newStore(t)

		err := store.Put(context.Background(), "objects/ab/cdef", func(destination io.Writer) error {
			_, _ = io.WriteString(destination, "partial")
			return errWriteFailed
		})

		require.ErrorIs(t, err, errWriteFailed)
		exists, err := store.Exists(context.Background(), "objects/ab/cdef")
		require.NoError(t, err)
		require.False(t, exists)
		keys, err := store.List(context.Background(), "objects")
		require.NoError(t, err)
		require.Empty(t, keys)
	})

	t.Run(`Given a store with a blob,
	when Put is called again for the same key,
	then it succeeds and keeps the original content`, func(t *testing.T) {
		store := newStore(t)
		putString(t, store, "snapshots/id.json", "original")

		putString(t, store, "snapshots/id.json", "replacement")

		require.Equal(t, "original", getString(t, store, "snapshots/id.json"))
	})

	t.Run(`Given a store with blobs under several prefixes,
	when List is called with a prefix,
	then it returns only the keys under that prefix, recursively and sorted`, func(t *testing.T) {
		store := newStore(t)
		putString(t, store, "objects/encrypted/1/cd/ef", "c")
		putString(t, store, "objects/ab/cd", "a")
		putString(t, store, "objects/encrypted/1/ab/cd", "b")
		putString(t, store, "objectsx/ab/cd", "x")
		putString(t, store, "snapshots/id.json", "s")

		keys, err := store.List(context.Background(), "objects/encrypted/1")
		require.NoError(t, err)
		require.Equal(t, []string{"objects/encrypted/1/ab/cd", "objects/encrypted/1/cd/ef"}, keys)

		keys, err = store.List(context.Background(), "objects")
		require.NoError(t, err)
		require.Equal(t, []string{"objects/ab/cd", "objects/encrypted/1/ab/cd", "objects/encrypted/1/cd/ef"}, keys)
	})

	t.Run(`Given an empty store,
	when List is called with a missing prefix,
	then it returns no keys and no error`, func(t *testing.T) {
		store := newStore(t)

		keys, err := store.List(context.Background(), "objects/encrypted/9")

		require.NoError(t, err)
		require.Empty(t, keys)
	})

	t.Run(`Given a store with blobs under several prefixes,
	when DeletePrefix is called,
	then it removes only the blobs under that prefix`, func(t *testing.T) {
		store := newStore(t)
		putString(t, store, "objects/encrypted/1/ab/cd", "old")
		putString(t, store, "objects/encrypted/1/ef/01", "old")
		putString(t, store, "objects/encrypted/2/ab/cd", "new")

		require.NoError(t, store.DeletePrefix(context.Background(), "objects/encrypted/1"))

		keys, err := store.List(context.Background(), "objects")
		require.NoError(t, err)
		require.Equal(t, []string{"objects/encrypted/2/ab/cd"}, keys)
		require.NoError(t, store.DeletePrefix(context.Background(), "objects/encrypted/7"))
	})

	t.Run(`Given a store,
	when it is used with keys that escape the repository or name temporary files,
	then it returns an invalid key error`, func(t *testing.T) {
		store := newStore(t)

		for _, key := range []string{"", "../config.json", "/etc/passwd", "objects/../../x", "objects/.tmp", "objects//ab", `objects\ab`} {
			err := store.Put(context.Background(), key, func(io.Writer) error { return nil })
			require.ErrorIs(t, err, blob.ErrInvalidKey, key)
			_, err = store.Get(context.Background(), key)
			require.ErrorIs(t, err, blob.ErrInvalidKey, key)
		}
	})

	t.Run(`Given a store and a canceled context,
	when Put is called,
	then it returns a context canceled error and stores nothing`, func(t *testing.T) {
		store := newStore(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := store.Put(ctx, "objects/ab/cdef", func(destination io.Writer) error {
			_, err := io.WriteString(destination, "content")
			return err
		})

		require.ErrorIs(t, err, context.Canceled)
		exists, err := store.Exists(context.Background(), "objects/ab/cdef")
		require.NoError(t, err)
		require.False(t, exists)
	})
}
