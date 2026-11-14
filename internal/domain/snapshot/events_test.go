package snapshot_test

import (
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestSnapshotEvents(t *testing.T) {
	t.Parallel()

	t.Run(`Given a saved manifest,
	when NewSnapshotCreated is called,
	then the event belongs to the snapshot and carries what was saved`, func(t *testing.T) {
		t.Parallel()

		manifest := fixtures.ManifestBuilder{ID: new("snapshot-id")}.Build()

		skipped := []snapshot.SkippedFile{snapshot.NewSkippedFile("link", snapshot.SymlinkKind)}

		created := snapshot.NewSnapshotCreated(&manifest, 3, 42, skipped)

		require.Equal(t, snapshot.SnapshotCreatedEventName, created.EventName())
		require.Equal(t, "snapshot-id", created.SnapshotID())
		require.Equal(t, 3, created.FileCount())
		require.Equal(t, int64(42), created.TotalSize())
		require.Equal(t, skipped, created.SkippedFiles())
		require.NotEmpty(t, created.EventID())
	})

	t.Run(`Given a restore result with a failed file,
	when Restored is called,
	then the event is unsuccessful and lists the failed file`, func(t *testing.T) {
		t.Parallel()

		result := snapshot.NewResult("snapshot-id")
		result.AddSuccess()
		result.AddFailed(*snapshot.NewFailedResult("a", "b"))

		restored := result.Restored()

		require.Equal(t, snapshot.SnapshotRestoredEventName, restored.EventName())
		require.Equal(t, "snapshot-id", restored.SnapshotID())
		require.Equal(t, 2, restored.TotalFiles())
		require.Equal(t, []snapshot.FailedResult{*snapshot.NewFailedResult("a", "b")}, restored.Failed())
		require.False(t, restored.IsSuccess())
	})

	t.Run(`Given a restored file,
	when NewFileRestored is called,
	then the event belongs to the snapshot and names the file`, func(t *testing.T) {
		t.Parallel()

		restored := snapshot.NewFileRestored("snapshot-id", "/source/a")

		require.Equal(t, snapshot.FileRestoredEventName, restored.EventName())
		require.Equal(t, "snapshot-id", restored.SnapshotID())
		require.Equal(t, "/source/a", restored.FilePath())
	})

	t.Run(`Given a retention decision that does not keep a snapshot,
	when Forgotten is called,
	then the event describes the forgotten snapshot`, func(t *testing.T) {
		t.Parallel()

		policy, err := snapshot.NewRetentionPolicy(snapshot.RetentionRules{Last: 1})
		require.NoError(t, err)
		decisions := policy.Apply([]snapshot.ManifestResume{
			resumeAt("new", "/home", "2026-11-14T10:00:00Z"),
			resumeAt("old", "/home", "2026-11-13T10:00:00Z"),
		}, time.Now())

		forgotten := decisions[1].Forgotten()

		require.Equal(t, snapshot.SnapshotForgottenEventName, forgotten.EventName())
		require.Equal(t, "old", forgotten.SnapshotID())
		require.Equal(t, "/home", forgotten.Source())
		require.Equal(t, decisions[1].Snapshot().CreatedAt(), forgotten.CreatedAt())
	})

	t.Run(`Given a prune plan with unreferenced and missing objects,
	when Pruned and MissingDetected are called,
	then the events belong to the repository and carry the objects`, func(t *testing.T) {
		t.Parallel()

		manifests := []snapshot.Manifest{fixtures.ManifestBuilder{Files: []snapshot.File{
			fixtures.FileBuilder{Hash: new("aa01")}.Build(),
		}}.Build()}
		plan := snapshot.NewPrunePlan(manifests, []string{"bb02", "cc03"})

		pruned := plan.Pruned("repository-id")
		missing := plan.MissingDetected("repository-id")

		require.Equal(t, snapshot.ObjectsPrunedEventName, pruned.EventName())
		require.Equal(t, "repository-id", pruned.RepositoryID())
		require.Equal(t, 2, pruned.Removed())
		require.Equal(t, snapshot.MissingObjectsDetectedEventName, missing.EventName())
		require.Equal(t, "repository-id", missing.RepositoryID())
		require.Equal(t, []string{"aa01"}, missing.Hashes())
	})

	t.Run(`Given a prune plan with orphan key generations,
	when GenerationsRemoved is called,
	then the event belongs to the repository and carries the generations`, func(t *testing.T) {
		t.Parallel()

		plan := snapshot.NewPrunePlan(nil, nil)
		plan.RecordOrphanGenerations([]uint64{1, 2})

		removed := plan.GenerationsRemoved("repository-id")

		require.Equal(t, snapshot.GenerationsRemovedEventName, removed.EventName())
		require.Equal(t, "repository-id", removed.RepositoryID())
		require.Equal(t, []uint64{1, 2}, removed.Generations())
	})

	t.Run(`Given removed temporary files,
	when NewTemporariesRemoved is called,
	then the event belongs to the repository and carries the count`, func(t *testing.T) {
		t.Parallel()

		removed := snapshot.NewTemporariesRemoved("repository-id", 3)

		require.Equal(t, snapshot.TemporariesRemovedEventName, removed.EventName())
		require.Equal(t, "repository-id", removed.RepositoryID())
		require.Equal(t, 3, removed.Removed())
	})
}
