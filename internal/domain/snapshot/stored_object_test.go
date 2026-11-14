package snapshot_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/stretchr/testify/require"
)

func TestStoredObject(t *testing.T) {
	t.Parallel()

	t.Run(`Given stored objects,
	when their hash, size and hashes are read,
	then they keep the values they were created with`, func(t *testing.T) {
		t.Parallel()

		objects := []snapshot.StoredObject{snapshot.NewStoredObject("aa01", 12), snapshot.NewStoredObject("bb02", 0)}

		require.Equal(t, "aa01", objects[0].Hash())
		require.Equal(t, int64(12), objects[0].Size())
		require.Equal(t, []string{"aa01", "bb02"}, snapshot.StoredHashes(objects))
	})
}
