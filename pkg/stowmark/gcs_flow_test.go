//go:build integration

package stowmark_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGCSRepositoryFlow(t *testing.T) {
	for k, version := range Versions() {
		t.Run(`Running GCS workflow, with format version `+k, func(t *testing.T) {
			ctx := t.Context()

			gcsContainer := startGCSContainer(t, ctx)
			endpoint := gcsContainerEndpoint(t, ctx, gcsContainer)

			createGCSBucket(t, ctx, endpoint, "stowmark")

			t.Setenv("STOWMARK_GCS_ENDPOINT", endpoint)

			repositoryURL := "gcs://stowmark/backups"

			testRoot := t.TempDir()
			sourcePath := filepath.Join(testRoot, "source")
			restorePath := filepath.Join(testRoot, "restore")

			require.NoError(t, os.MkdirAll(sourcePath, 0o755))
			createSourceFixture(t, sourcePath)

			mainFlow(t, ctx, Folders{
				Source:     sourcePath,
				Repository: repositoryURL,
				Restore:    &restorePath,
			}, version)
		})
	}
}
