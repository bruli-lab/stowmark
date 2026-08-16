//go:build integration

package stowmark_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWebdavRepositoryFlow(t *testing.T) {
	for k, version := range Versions() {
		t.Run(`Running WEBDAV workflow, with format version `+k, func(t *testing.T) {
			ctx := t.Context()

			container := startWebDAVContainer(t, ctx)
			address := webDAVContainerAddress(t, ctx, container)

			t.Setenv("STOWMARK_WEBDAV_USERNAME", "stowmark")
			t.Setenv("STOWMARK_WEBDAV_PASSWORD", "stowmark")

			repositoryURL := fmt.Sprintf("webdav://%s/backups", address)

			testRoot := t.TempDir()
			sourcePath := filepath.Join(testRoot, "source")
			restorePath := filepath.Join(testRoot, "restore")

			require.NoError(t, os.MkdirAll(sourcePath, 0o755))
			createSourceFixture(t, sourcePath)

			mainFlow(t, ctx, Folders{
				Source:     sourcePath,
				Repository: repositoryURL,
				Restore:    new(restorePath),
			}, version)
		})
	}
}
