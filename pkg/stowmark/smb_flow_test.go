//go:build integration

package stowmark_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSMBRepositoryFlow(t *testing.T) {
	ctx := context.Background()

	container := startSMBContainer(t, ctx)

	address := smbContainerAddress(t, ctx, container)

	t.Setenv("STOWMARK_SMB_PASSWORD", "stowmark")

	repositoryURL := fmt.Sprintf("smb://stowmark@%s/stowmark/backups", address)

	testRoot := t.TempDir()
	sourcePath := filepath.Join(testRoot, "source")
	restorePath := filepath.Join(testRoot, "restore")

	require.NoError(t, os.MkdirAll(sourcePath, 0o755))
	createSourceFixture(t, sourcePath)

	mainFlow(t, ctx, Folders{
		Source:     sourcePath,
		Repository: repositoryURL,
		Restore:    new(restorePath),
	})
}
