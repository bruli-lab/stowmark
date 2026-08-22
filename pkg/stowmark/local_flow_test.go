//go:build integration

package stowmark_test

import (
	"path/filepath"
	"testing"
)

func TestLocalRepositoryFlow(t *testing.T) {
	for k, version := range Versions() {
		t.Run(`Running local workflow, with format version `+k, func(t *testing.T) {
			testRoot := t.TempDir()
			sourcePath := filepath.Join(testRoot, "source-"+k)
			repositoryPath := filepath.Join(testRoot, "repository-"+k)
			restorePath := filepath.Join(testRoot, "restore-"+k)
			alternativeRestorePath := filepath.Join(testRoot, "alternative-restore-"+k)
			keyPairsFolder := filepath.Join(testRoot, "keypairs-"+k)
			newKeyPairsFolder := filepath.Join(testRoot, "new-keypairs-"+k)
			fold := Folders{
				Source:             sourcePath,
				Repository:         repositoryPath,
				Restore:            new(restorePath),
				keyPairsFolder:     keyPairsFolder,
				newKeyPairsFolder:  newKeyPairsFolder,
				AlternativeRestore: new(alternativeRestorePath),
			}
			mainFlow(t, t.Context(), fold, version)
		})
	}
}
