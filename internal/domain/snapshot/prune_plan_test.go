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

func TestManifest_ObjectSizes(t *testing.T) {
	t.Parallel()

	t.Run(`Given a manifest with a small file and a chunked file,
	when ObjectSizes method is called,
	then it returns the size of the small file and of every chunk`, func(t *testing.T) {
		t.Parallel()

		manifest := fixtures.ManifestBuilder{Files: []snapshot.File{
			fixtures.FileBuilder{Hash: new("aa01"), Size: new(int64(10))}.Build(),
			fixtures.FileBuilder{Hash: new("bb02"), Size: new(int64(7)), Chunks: []snapshot.Chunk{
				*snapshot.NewChunk("cc03", 0, 4),
				*snapshot.NewChunk("dd04", 4, 3),
			}}.Build(),
		}}.Build()

		require.Equal(t, map[string]int64{"aa01": 10, "cc03": 4, "dd04": 3}, manifest.ObjectSizes())
	})
}

func TestNewPrunePlan(t *testing.T) {
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

	tests := []struct {
		name                 string
		manifests            []snapshot.Manifest
		stored               []string
		expectedStored       int
		expectedReferenced   int
		expectedUnreferenced []string
		expectedMissing      []string
	}{
		{
			name:                 "with manifests that share objects and duplicated stored objects, then it counts each object once",
			manifests:            manifests,
			stored:               []string{"ff09", "aa01", "bb02", "ee08", "ff09"},
			expectedStored:       4,
			expectedReferenced:   3,
			expectedUnreferenced: []string{"ee08", "ff09"},
			expectedMissing:      []string{"cc03"},
		},
		{
			name:                 "without manifests, then every stored object is unreferenced",
			stored:               []string{"bb02", "aa01"},
			expectedStored:       2,
			expectedUnreferenced: []string{"aa01", "bb02"},
			expectedMissing:      []string{},
		},
		{
			name:                 "with every object referenced and stored, then there is nothing to prune",
			manifests:            manifests,
			stored:               []string{"aa01", "bb02", "cc03"},
			expectedStored:       3,
			expectedReferenced:   3,
			expectedUnreferenced: []string{},
			expectedMissing:      []string{},
		},
	}
	for _, tt := range tests {
		t.Run(`Given manifests and stored objects,
		when NewPrunePlan is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			plan := snapshot.NewPrunePlan(tt.manifests, tt.stored)

			require.Equal(t, tt.expectedStored, plan.StoredObjects())
			require.Equal(t, tt.expectedReferenced, plan.ReferencedObjects())
			require.Equal(t, tt.expectedUnreferenced, plan.Unreferenced())
			require.Equal(t, tt.expectedMissing, plan.Missing())
		})
	}
}
