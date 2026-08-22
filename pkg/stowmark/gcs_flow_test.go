//go:build integration

package stowmark_test

import (
	"path/filepath"
	"testing"
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
			alternativeRestorePath := filepath.Join(testRoot, "alternaive-restore")
			keyPairsFolder := filepath.Join(testRoot, "keypairs")
			newKeyPairsFolder := filepath.Join(testRoot, "new-keypairs")

			mainFlow(t, ctx, Folders{
				Source:             sourcePath,
				Repository:         repositoryURL,
				keyPairsFolder:     keyPairsFolder,
				Restore:            &restorePath,
				AlternativeRestore: new(alternativeRestorePath),
				newKeyPairsFolder:  newKeyPairsFolder,
			}, version)
		})
	}
}
