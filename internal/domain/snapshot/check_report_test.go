package snapshot_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func manifestReferencing(id string, hashes ...string) snapshot.Manifest {
	files := make([]snapshot.File, len(hashes))
	for i := range hashes {
		files[i] = fixtures.FileBuilder{Hash: &hashes[i]}.Build()
	}
	return fixtures.ManifestBuilder{ID: &id, Files: files}.Build()
}

func TestNewCheckReport(t *testing.T) {
	t.Parallel()

	t.Run(`Given manifests with objects of known sizes,
	when ObjectSize is called,
	then it returns the size of a referenced object and zero for an unknown one`, func(t *testing.T) {
		t.Parallel()

		manifest := fixtures.ManifestBuilder{Files: []snapshot.File{
			fixtures.FileBuilder{Hash: new("aa01"), Size: new(int64(42))}.Build(),
		}}.Build()

		report := snapshot.NewCheckReport([]snapshot.Manifest{manifest}, nil, []string{"aa01"})

		require.Equal(t, int64(42), report.ObjectSize("aa01"))
		require.Zero(t, report.ObjectSize("ff99"))
	})

	t.Run(`Given manifests, a corrupt manifest and stored objects with orphans and gaps,
	when NewCheckReport is called,
	then it reports the missing objects with the snapshots that use them, the orphans and the readable objects`, func(t *testing.T) {
		t.Parallel()

		manifests := []snapshot.Manifest{
			manifestReferencing("s1", "aa01", "bb02", "cc03"),
			manifestReferencing("s2", "bb02", "cc03", "dd04"),
		}
		corrupt := []snapshot.ManifestProblem{snapshot.NewManifestProblem("s3", "invalid json")}
		stored := []string{"ee05", "aa01", "bb02", "aa01"}

		report := snapshot.NewCheckReport(manifests, corrupt, stored)

		require.Equal(t, 2, report.Snapshots())
		require.Equal(t, corrupt, report.CorruptManifests())
		require.Equal(t, 3, report.StoredObjects())
		require.Equal(t, 4, report.ReferencedObjects())
		require.Equal(t, 1, report.UnreferencedObjects())
		require.Len(t, report.MissingObjects(), 2)
		require.Equal(t, "cc03", report.MissingObjects()[0].Hash())
		require.Equal(t, []string{"s1", "s2"}, report.MissingObjects()[0].Snapshots())
		require.Equal(t, "dd04", report.MissingObjects()[1].Hash())
		require.Equal(t, []string{"s2"}, report.MissingObjects()[1].Snapshots())
		require.Equal(t, []string{"aa01", "bb02"}, report.ReadableObjects())
		require.True(t, report.HasErrors())
		require.False(t, report.ReadData())
	})

	t.Run(`Given a healthy repository with warnings,
	when the stale locks, orphan generations and data results are recorded,
	then the report keeps them and only corrupt data makes it fail`, func(t *testing.T) {
		t.Parallel()

		report := snapshot.NewCheckReport([]snapshot.Manifest{manifestReferencing("s1", "aa01")}, nil, []string{"aa01", "bb02"})
		report.RecordStaleLocks(2)
		report.RecordOrphanGenerations([]uint64{1})

		require.False(t, report.HasErrors(), "orphan objects, stale locks and old generations are warnings")
		require.Equal(t, 2, report.StaleLocks())
		require.Equal(t, []uint64{1}, report.OrphanGenerations())

		report.RecordData(1, []snapshot.ObjectProblem{snapshot.NewObjectProblem("ff02", "b"), snapshot.NewObjectProblem("aa01", "hash mismatch")})

		require.True(t, report.ReadData())
		require.Equal(t, 1, report.ReadObjects())
		require.Equal(t, "aa01", report.CorruptObjects()[0].Hash())
		require.Equal(t, "hash mismatch", report.CorruptObjects()[0].Reason())
		require.True(t, report.HasErrors())
	})
}
