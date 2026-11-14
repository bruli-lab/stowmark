package stowmark_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bruli-lab/stowmark/pkg/stowmark"
	"github.com/stretchr/testify/require"
)

func TestHandler_Init(t *testing.T) {
	t.Run(`Given a Handler for a local repository whose path is an existing file,
	when Init method is called,
	then it returns the underlying error instead of an unexpected events error`, func(t *testing.T) {
		repositoryPath := filepath.Join(t.TempDir(), "repository")
		require.NoError(t, os.WriteFile(repositoryPath, []byte("not a directory"), 0o600))

		handler, err := stowmark.NewHandler(t.Context(), repositoryPath)
		require.NoError(t, err)
		defer func() {
			_ = handler.Close()
		}()

		err = handler.Init(t.Context(), &stowmark.Compression{Type: "none"}, 0, nil, false)

		require.Error(t, err)
		require.NotContains(t, err.Error(), "unexpected number of init events")
	})
}
