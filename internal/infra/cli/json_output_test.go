package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func runCLI(t *testing.T, args ...string) []byte {
	t.Helper()

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	require.NoError(t, root.ExecuteContext(t.Context()))
	return stdout.Bytes()
}

func decode[T any](t *testing.T, output []byte) T {
	t.Helper()

	var document T
	require.NoError(t, json.Unmarshal(output, &document), string(output))
	return document
}

func TestCLI_JSONOutput(t *testing.T) {
	t.Parallel()

	t.Run(`Given a local repository and a source directory,
	when the commands are run with --json,
	then each one prints a single JSON document with its result on stdout`, func(t *testing.T) {
		t.Parallel()

		repo := filepath.Join(t.TempDir(), "repo")
		source := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(source, "a.txt"), []byte("hello"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(source, "skip.log"), []byte("skipped"), 0o600))

		initialized := decode[initDocument](t, runCLI(t, "init", "--repo", repo, "--version", "3", "--json"))
		require.NotEmpty(t, initialized.RepositoryID)
		require.Empty(t, initialized.Warnings)

		created := decode[createDocument](t, runCLI(t, "snapshot", "create", source, "--repo", repo, "--exclude", "*.log", "--json"))
		require.NotEmpty(t, created.SnapshotID)
		require.Equal(t, 1, created.Files)
		require.Equal(t, int64(5), created.Size)

		listed := decode[[]snapshotDocument](t, runCLI(t, "snapshot", "list", "--repo", repo, "--json"))
		require.Len(t, listed, 1)
		require.Equal(t, created.SnapshotID, listed[0].ID)

		manifest := decode[manifestDocument](t, runCLI(t, "snapshot", "get", "--repo", repo, "--id", created.SnapshotID, "--json"))
		require.Equal(t, created.SnapshotID, manifest.ID)
		require.Len(t, manifest.Entries, 1)
		require.Equal(t, "file", manifest.Entries[0].Kind)
		require.Equal(t, "0600", *manifest.Entries[0].Mode)

		verified := decode[resultDocument](t, runCLI(t, "snapshot", "verify", "--repo", repo, "--id", created.SnapshotID, "--json"))
		require.True(t, verified.Success)
		require.Equal(t, 1, verified.Valid)

		restored := decode[resultDocument](t, runCLI(t, "snapshot", "restore", "--repo", repo, "--id", created.SnapshotID,
			"--destination", t.TempDir(), "--json"))
		require.True(t, restored.Success)

		checked := decode[checkDocument](t, runCLI(t, "check", "--repo", repo, "--read-data", "--json"))
		require.True(t, checked.Success)
		require.Equal(t, 1, checked.ReadObjects)

		plan := decode[[]retentionDocument](t, runCLI(t, "forget", "--repo", repo, "--keep-last", "1", "--dry-run", "--json"))
		require.Len(t, plan, 1)
		require.True(t, plan[0].Keep)

		forgotten := decode[forgetDocument](t, runCLI(t, "forget", "--repo", repo, "--keep-within", "1h", "--prune", "--json"))
		require.Empty(t, forgotten.Forgotten)
		require.NotNil(t, forgotten.Prune)

		prunePlan := decode[prunePlanDocument](t, runCLI(t, "prune", "--repo", repo, "--dry-run", "--json"))
		require.Equal(t, 1, prunePlan.StoredObjects)
		require.Zero(t, prunePlan.UnreferencedBytes)

		pruned := decode[pruneDocument](t, runCLI(t, "prune", "--repo", repo, "--json"))
		require.Zero(t, pruned.RemovedObjects)

		unlocked := decode[unlockDocument](t, runCLI(t, "unlock", "--repo", repo, "--json"))
		require.Equal(t, unlockDocument{}, unlocked)
	})

	t.Run(`Given a local repository,
	when a command is run without --json,
	then it prints the text output`, func(t *testing.T) {
		t.Parallel()

		repo := filepath.Join(t.TempDir(), "repo")
		runCLI(t, "init", "--repo", repo)

		output := runCLI(t, "snapshot", "list", "--repo", repo)

		require.Equal(t, "No snapshots found.\n", string(output))
	})
}
