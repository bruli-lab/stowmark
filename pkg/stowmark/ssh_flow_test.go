package stowmark_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSHRepositoryFlow(t *testing.T) {
	ctx := context.Background()

	keyPair := createSSHKeyPair(t)
	container := startSSHContainer(t, ctx, keyPair.publicKey)

	address := sshContainerAddress(t, ctx, container)

	createKnownHosts(t, address)
	t.Setenv(
		"STOWMARK_SSH_PRIVATE_KEY",
		keyPair.privateKeyPath,
	)

	repositoryURL := fmt.Sprintf("ssh://stowmark@%s/repositories/backups", address)

	testRoot := t.TempDir()
	sourcePath := filepath.Join(testRoot, "source")
	restorePath := filepath.Join(testRoot, "restore")

	require.NoError(t, os.MkdirAll(sourcePath, 0o755))
	createSourceFixture(t, sourcePath)

	mainFlow(t, Folders{
		Source:     sourcePath,
		Repository: repositoryURL,
		Restore:    new(restorePath),
	})
}
