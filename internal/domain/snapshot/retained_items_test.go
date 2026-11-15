package snapshot_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestRetainedItems(t *testing.T) {
	t.Parallel()

	t.Run(`Given two manifests that share an object,
	when RetainedItems is called,
	then it returns the config, every manifest and each referenced object once`, func(t *testing.T) {
		t.Parallel()

		first := fixtures.ManifestBuilder{ID: new("s1"), Files: []snapshot.File{
			fixtures.FileBuilder{Hash: new("bb02")}.Build(),
			fixtures.FileBuilder{Hash: new("aa01")}.Build(),
		}}.Build()
		second := fixtures.ManifestBuilder{ID: new("s2"), Files: []snapshot.File{
			fixtures.FileBuilder{Hash: new("aa01")}.Build(),
		}}.Build()

		conf := fixtures.ConfigBuilder{}.Build(t)

		items := snapshot.RetainedItems([]snapshot.Manifest{first, second}, &conf)

		require.Equal(t, []repository.RetainedItem{
			repository.NewRetainedConfig(),
			repository.NewRetainedManifest("s1"),
			repository.NewRetainedManifest("s2"),
			repository.NewRetainedObject("aa01", false, 0),
			repository.NewRetainedObject("bb02", false, 0),
		}, items)
	})

	t.Run(`Given an encrypted repository,
	when RetainedItems is called,
	then the objects are located in the current key generation`, func(t *testing.T) {
		t.Parallel()

		manifest := fixtures.ManifestBuilder{Files: []snapshot.File{fixtures.FileBuilder{Hash: new("aa01")}.Build()}}.Build()
		conf := fixtures.ConfigBuilder{EncryptionConfig: encryption.NewEncryptionConfig("key", "fingerprint", 5)}.Build(t)

		items := snapshot.RetainedItems([]snapshot.Manifest{manifest}, &conf)

		require.Equal(t, repository.NewRetainedObject("aa01", true, 5), items[len(items)-1])
	})
}
