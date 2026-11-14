package snapshot_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestManifest_ReferencedHashes(t *testing.T) {
	t.Parallel()

	t.Run(`Given a manifest with a small file and a chunked file,
	when ReferencedHashes method is called,
	then it returns the small file hash and the chunk hashes, not the chunked file hash`, func(t *testing.T) {
		t.Parallel()

		manifest := fixtures.ManifestBuilder{Files: []snapshot.File{
			fixtures.FileBuilder{Hash: new("aa01")}.Build(),
			fixtures.FileBuilder{Hash: new("bb02"), Chunks: []snapshot.Chunk{
				*snapshot.NewChunk("cc03", 0, 4),
				*snapshot.NewChunk("dd04", 4, 4),
			}}.Build(),
		}}.Build()

		require.Equal(t, []string{"aa01", "cc03", "dd04"}, manifest.ReferencedHashes())
	})
}

func TestReferences(t *testing.T) {
	t.Parallel()

	manifests := []snapshot.Manifest{
		fixtures.ManifestBuilder{Files: []snapshot.File{
			fixtures.FileBuilder{Hash: new("aa01")}.Build(),
			fixtures.FileBuilder{Hash: new("bb02")}.Build(),
		}}.Build(),
		fixtures.ManifestBuilder{Files: []snapshot.File{
			fixtures.FileBuilder{Hash: new("bb02")}.Build(),
			fixtures.FileBuilder{Hash: new("cc03")}.Build(),
		}}.Build(),
	}
	stored := []string{"ff09", "aa01", "bb02", "ee08", "ff09"}

	t.Run(`Given references built from manifests that share objects,
	when Len and Contains are called,
	then each hash is counted once`, func(t *testing.T) {
		t.Parallel()

		references := snapshot.NewReferences(manifests)

		require.Equal(t, 3, references.Len())
		require.True(t, references.Contains("cc03"))
		require.False(t, references.Contains("ee08"))
	})

	t.Run(`Given references built from manifests,
	when Unreferenced is called with the stored objects,
	then it returns the sorted stored objects no manifest uses`, func(t *testing.T) {
		t.Parallel()

		require.Equal(t, []string{"ee08", "ff09"}, snapshot.NewReferences(manifests).Unreferenced(stored))
	})

	t.Run(`Given references built from manifests,
	when Missing is called with the stored objects,
	then it returns the referenced objects that are not stored`, func(t *testing.T) {
		t.Parallel()

		require.Equal(t, []string{"cc03"}, snapshot.NewReferences(manifests).Missing(stored))
	})

	t.Run(`Given references without manifests,
	when Unreferenced is called,
	then every stored object is unreferenced`, func(t *testing.T) {
		t.Parallel()

		require.Equal(t, []string{"aa01", "bb02", "ee08", "ff09"}, snapshot.NewReferences(nil).Unreferenced(stored))
	})
}
