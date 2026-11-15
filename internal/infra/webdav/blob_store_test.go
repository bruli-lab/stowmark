package webdav_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bruli-lab/stowmark/internal/infra/webdav"
	"github.com/stretchr/testify/require"
	"github.com/studio-b12/gowebdav"
	xwebdav "golang.org/x/net/webdav"
)

func TestBlobStore_ConcurrentPut(t *testing.T) {
	t.Parallel()

	t.Run(`Given a WebDAV server that locks a collection while it is being created,
	when many objects of the same directory are put concurrently,
	then every put succeeds and the directory is created only once`, func(t *testing.T) {
		t.Parallel()

		var collections atomic.Int64
		handler := &xwebdav.Handler{FileSystem: xwebdav.NewMemFS(), LockSystem: xwebdav.NewMemLS()}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "MKCOL" {
				collections.Add(1)
			}
			handler.ServeHTTP(w, r)
		}))
		t.Cleanup(server.Close)
		store := webdav.NewBlobStore(gowebdav.NewClient(server.URL, "", ""), "repo")

		const objects = 64
		errs := make([]error, objects)
		var group sync.WaitGroup
		for i := range objects {
			group.Go(func() {
				key := fmt.Sprintf("objects/ab/%062x", i)
				errs[i] = store.Put(t.Context(), key, func(w io.Writer) error {
					_, err := fmt.Fprintf(w, "object %d", i)
					return err
				})
			})
		}
		group.Wait()

		for _, err := range errs {
			require.NoError(t, err)
		}
		entries, err := store.List(t.Context(), "objects")
		require.NoError(t, err)
		require.Len(t, entries, objects)
		require.LessOrEqual(t, collections.Load(), int64(4))
	})
}
