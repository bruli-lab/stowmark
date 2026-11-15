package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

var snapshotIDPattern = regexp.MustCompile(`Snapshot created: (\S+)`)

func createdSnapshotID(t *testing.T, output []byte) string {
	t.Helper()
	match := snapshotIDPattern.FindSubmatch(output)
	require.NotNil(t, match, string(output))
	return string(match[1])
}

func generateKeys(t *testing.T) keyPairDocument {
	t.Helper()
	return decode[keyPairDocument](t, runCLI(t, "key", "generate", "--folder", t.TempDir(), "--json"))
}

func TestCLI_TextOutput(t *testing.T) {
	t.Parallel()

	t.Run(`Given an encrypted format version 3 repository and a source directory,
	when every command is run with text output,
	then each one prints its human readable result`, func(t *testing.T) {
		t.Parallel()

		keys := generateKeys(t)
		repo := filepath.Join(t.TempDir(), "repo")
		source := t.TempDir()
		file := filepath.Join(source, "a.txt")
		require.NoError(t, os.WriteFile(file, []byte("first version"), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(source, "empty"), 0o750))
		if runtime.GOOS != "windows" {
			require.NoError(t, os.Symlink("a.txt", filepath.Join(source, "link")))
		}

		require.Contains(t, string(runCLI(t, "init", "--repo", repo, "--version", "3", "--public-key", keys.PublicKey)), "Initialized Stowmark repository")

		first := createdSnapshotID(t, runCLI(t, "snapshot", "create", source, "--repo", repo, "--private-key", keys.PrivateKey))
		require.NoError(t, os.WriteFile(file, []byte("second version"), 0o600))
		second := createdSnapshotID(t, runCLI(t, "snapshot", "create", source, "--repo", repo, "--private-key", keys.PrivateKey))

		listed := string(runCLI(t, "snapshot", "list", "--repo", repo, "--private-key", keys.PrivateKey))
		require.Contains(t, listed, "ID")
		require.Contains(t, listed, first)
		require.Contains(t, listed, second)

		manifest := string(runCLI(t, "snapshot", "get", "--repo", repo, "--id", second, "--private-key", keys.PrivateKey))
		require.Contains(t, manifest, "SOURCE:")
		require.Contains(t, manifest, file)
		require.Contains(t, manifest, filepath.Join(source, "empty")+"/")
		if runtime.GOOS != "windows" {
			require.Contains(t, manifest, filepath.Join(source, "link")+" -> a.txt")
		}

		verified := string(runCLI(t, "snapshot", "verify", "--repo", repo, "--id", second, "--private-key", keys.PrivateKey))
		require.Contains(t, verified, "SNAPSHOT:")
		require.Contains(t, verified, "INVALID:")

		destination := t.TempDir()
		restored := string(runCLI(t, "snapshot", "restore", "--repo", repo, "--id", second, "--destination", destination, "--private-key", keys.PrivateKey))
		require.Contains(t, restored, second)
		content, err := os.ReadFile(filepath.Join(destination, "a.txt"))
		require.NoError(t, err)
		require.Equal(t, "second version", string(content))

		fileDestination := t.TempDir()
		restoredFile := string(runCLI(t, "snapshot", "restore", "--repo", repo, "--id", first, "--file", file, "--destination", fileDestination, "--private-key", keys.PrivateKey))
		require.Contains(t, restoredFile, "Restored file")

		checked := string(runCLI(t, "check", "--repo", repo, "--read-data", "--private-key", keys.PrivateKey))
		require.Contains(t, checked, "Repository OK")

		plan := string(runCLI(t, "forget", "--repo", repo, "--keep-last", "1", "--dry-run", "--private-key", keys.PrivateKey))
		require.Contains(t, plan, "Would keep 1 snapshot(s) and forget 1 snapshot(s)")

		forgotten := string(runCLI(t, "forget", "--repo", repo, "--keep-last", "1", "--private-key", keys.PrivateKey))
		require.Contains(t, forgotten, "FORGOTTEN")
		require.Contains(t, forgotten, "Forgot 1 snapshot(s)")

		prunePlan := string(runCLI(t, "prune", "--repo", repo, "--dry-run", "--private-key", keys.PrivateKey))
		require.Contains(t, prunePlan, "WOULD REMOVE:")
		require.Contains(t, prunePlan, "OLD KEY GENERATIONS:  none")

		pruned := string(runCLI(t, "prune", "--repo", repo, "--private-key", keys.PrivateKey))
		require.Contains(t, pruned, "Removed 1 unreferenced object(s)")

		newKeys := generateKeys(t)
		require.Contains(t, string(runCLI(t, "key", "rekey", "--repo", repo, "--private-key", keys.PrivateKey, "--public-key", newKeys.PublicKey)), "successful")
		rewrappedKeys := generateKeys(t)
		require.Contains(t, string(runCLI(t, "key", "rewrap", "--repo", repo, "--old-private-key", newKeys.PrivateKey, "--new-public-key", rewrappedKeys.PublicKey)), "successful")

		afterRotation := string(runCLI(t, "check", "--repo", repo, "--read-data", "--private-key", rewrappedKeys.PrivateKey))
		require.Contains(t, afterRotation, "Repository OK")
	})

	t.Run(`Given a repository with an orphan object and a missing object,
	when check and prune are run with text output,
	then they print the warnings and errors`, func(t *testing.T) {
		t.Parallel()

		repo := filepath.Join(t.TempDir(), "repo")
		source := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(source, "a.txt"), []byte("kept"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(source, "b.txt"), []byte("lost"), 0o600))
		runCLI(t, "init", "--repo", repo)
		runCLI(t, "snapshot", "create", source, "--repo", repo)
		orphan := filepath.Join(repo, "objects", "ab", "cdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
		require.NoError(t, os.MkdirAll(filepath.Dir(orphan), 0o750))
		require.NoError(t, os.WriteFile(orphan, []byte("orphan"), 0o600))
		removeObjectOf(t, repo, source, "b.txt")

		err := runCLIError(t, "check", "--repo", repo)
		require.Error(t, err)

		pruned := string(runCLI(t, "prune", "--repo", repo))
		require.Contains(t, pruned, "Removed 1 unreferenced object(s)")
		require.Contains(t, pruned, "WARNING: 1 referenced object(s) are missing")
	})
}

func removeObjectOf(t *testing.T, repo, source, name string) {
	t.Helper()
	manifest := decode[manifestDocument](t, runCLI(t, "snapshot", "get", "--repo", repo, "--id", decode[[]snapshotDocument](t, runCLI(t, "snapshot", "list", "--repo", repo, "--json"))[0].ID, "--json"))
	for _, entry := range manifest.Entries {
		if entry.Path == filepath.Join(source, name) {
			require.NoError(t, os.Remove(filepath.Join(repo, "objects", entry.Hash[:2], entry.Hash[2:])))
			return
		}
	}
	t.Fatalf("no entry for %s", name)
}
