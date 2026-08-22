//go:build integration

package stowmark_test

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestWebdavRepositoryFlow(t *testing.T) {
	for k, version := range Versions() {
		t.Run(fmt.Sprintf("Running WEBDAV workflow, with format version %d", version), func(t *testing.T) {
			ctx := t.Context()

			container := startWebDAVContainer(t, ctx)
			address := webDAVContainerAddress(t, ctx, container)

			t.Setenv("STOWMARK_WEBDAV_USERNAME", "stowmark")
			t.Setenv("STOWMARK_WEBDAV_PASSWORD", "stowmark")

			repositoryURL := fmt.Sprintf("webdav://%s/backups", address)

			testRoot := t.TempDir()
			sourcePath := filepath.Join(testRoot, "source-"+k)
			restorePath := filepath.Join(testRoot, "restore-"+k)
			alternativeRestorePath := filepath.Join(testRoot, "alternative-restore-"+k)
			keyPairsFolder := filepath.Join(testRoot, "keypairs-"+k)
			newKeyPairsFolder := filepath.Join(testRoot, "new-keypairs-"+k)

			mainFlow(t, ctx, Folders{
				Source:             sourcePath,
				Repository:         repositoryURL,
				Restore:            new(restorePath),
				keyPairsFolder:     keyPairsFolder,
				AlternativeRestore: new(alternativeRestorePath),
				newKeyPairsFolder:  newKeyPairsFolder,
			}, version)
		})
	}
}
