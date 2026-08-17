//go:build integration

package stowmark_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestSMBRepositoryFlow(t *testing.T) {
	for k, version := range Versions() {
		t.Run(`Running SMB workflow, with format version `+k, func(t *testing.T) {
			ctx := context.Background()

			container := startSMBContainer(t, ctx)

			address := smbContainerAddress(t, ctx, container)

			t.Setenv("STOWMARK_SMB_PASSWORD", "stowmark")

			repositoryURL := fmt.Sprintf("smb://stowmark@%s/stowmark/backups", address)

			testRoot := t.TempDir()
			sourcePath := filepath.Join(testRoot, "source")
			restorePath := filepath.Join(testRoot, "restore")
			alternativeRestorePath := filepath.Join(testRoot, "alternative-restore")
			keyPairsFolder := filepath.Join(testRoot, "keypairs")
			newKeyPairsFolder := filepath.Join(testRoot, "new-keypairs")

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
