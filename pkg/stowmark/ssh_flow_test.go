//go:build integration

package stowmark_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSHRepositoryFlow(t *testing.T) {
	for k, version := range Versions() {
		t.Run(`Running SSH workflow, with format version`+k, func(t *testing.T) {
			ctx := t.Context()

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

			mainFlow(t, ctx, Folders{
				Source:     sourcePath,
				Repository: repositoryURL,
				Restore:    new(restorePath),
			}, version)
		})
	}
}
