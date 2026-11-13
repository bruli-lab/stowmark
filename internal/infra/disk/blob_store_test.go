package disk_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/bruli-lab/stowmark/internal/infra/blob/blobtest"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/stretchr/testify/require"
)

func newBlobStore(t *testing.T) (store *disk.BlobStore, root string) {
	t.Helper()

	root = t.TempDir()
	store, err := disk.NewBlobStore(root)
	require.NoError(t, err)

	return store, root
}

func TestBlobStore_Contract(t *testing.T) {
	t.Parallel()

	blobtest.RunStoreContract(t, func(t *testing.T) blob.Store {
		store, _ := newBlobStore(t)
		return store
	})
}

func TestBlobStore_TemporaryFiles(t *testing.T) {
	t.Parallel()

	t.Run(`Given a disk BlobStore with a leftover temporary file from an interrupted write,
	when List is called,
	then it ignores the temporary file`, func(t *testing.T) {
		t.Parallel()

		store, root := newBlobStore(t)
		putBlob(t, store, "objects/ab/cdef", "content")
		leftover := filepath.Join(root, "objects", "ab", ".cdef.tmp-123")
		require.NoError(t, os.WriteFile(leftover, []byte("partial"), 0o600))

		keys, err := store.List(context.Background(), "objects")

		require.NoError(t, err)
		require.Equal(t, []string{"objects/ab/cdef"}, keys)
	})

	t.Run(`Given a disk BlobStore,
	when a blob is put,
	then it is stored under the repository root with the key as relative path`, func(t *testing.T) {
		t.Parallel()

		store, root := newBlobStore(t)

		putBlob(t, store, "snapshots/id.json", "manifest")

		data, err := os.ReadFile(filepath.Join(root, "snapshots", "id.json"))
		require.NoError(t, err)
		require.Equal(t, "manifest", string(data))
		entries, err := os.ReadDir(filepath.Join(root, "snapshots"))
		require.NoError(t, err)
		require.Len(t, entries, 1)
	})
}

func putBlob(t *testing.T, store blob.Store, key, content string) {
	t.Helper()

	require.NoError(t, store.Put(context.Background(), key, func(w io.Writer) error {
		_, err := io.WriteString(w, content)
		return err
	}))
}

func newObjectRepository(repositoryPath string) (*blob.ObjectRepository, error) {
	store, err := disk.NewBlobStore(repositoryPath)
	if err != nil {
		return nil, err
	}
	return blob.NewObjectRepository(store), nil
}

func newManifestRepository(repositoryPath string) (*blob.ManifestRepository, error) {
	store, err := disk.NewBlobStore(repositoryPath)
	if err != nil {
		return nil, err
	}
	return blob.NewManifestRepository(store), nil
}
