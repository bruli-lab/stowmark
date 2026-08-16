//go:build integration

package stowmark_test

import (
	"path/filepath"
	"testing"
)

func TestLocalRepositoryFlow(t *testing.T) {
	testRoot := t.TempDir()
	sourcePath := filepath.Join(testRoot, "source")
	repositoryPath := filepath.Join(testRoot, "repository")
	restorePath := filepath.Join(testRoot, "restore")
	fold := Folders{
		Source:     sourcePath,
		Repository: repositoryPath,
		Restore:    new(restorePath),
	}
	mainFlow(t, t.Context(), fold)
}
