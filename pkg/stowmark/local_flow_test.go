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
			sourcePath := filepath.Join(testRoot, "source-one")
			repositoryPath := filepath.Join(testRoot, "repository-"+k)
			restorePath := filepath.Join(testRoot, "restore-"+k)
			fold := Folders{
				Source:     sourcePath,
				Repository: repositoryPath,
				Restore:    new(restorePath),
			}
			mainFlow(t, t.Context(), fold, version)
		})
	}
}
