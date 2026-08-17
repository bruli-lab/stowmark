//go:build integration

package stowmark_test

import (
	"fmt"
	"path/filepath"
	"testing"
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
