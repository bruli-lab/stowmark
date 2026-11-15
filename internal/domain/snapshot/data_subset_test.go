package snapshot_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/stretchr/testify/require"
)

func TestParseDataSubset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value    string
		expected string
		valid    bool
	}{
		{value: "10%", expected: "10%", valid: true},
		{value: "0.5%", expected: "0.5%", valid: true},
		{value: "100%", expected: "100%", valid: true},
		{value: "2/7", expected: "2/7", valid: true},
		{value: "7/7", expected: "7/7", valid: true},
		{value: "0%"},
		{value: "101%"},
		{value: "abc%"},
		{value: "0/7"},
		{value: "8/7"},
		{value: "1/0"},
		{value: "a/7"},
		{value: "10"},
		{value: ""},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf(`Given the data subset %q,
		when ParseDataSubset is called,
		then it is valid: %t`, tt.value, tt.valid), func(t *testing.T) {
			t.Parallel()

			subset, err := snapshot.ParseDataSubset(tt.value)

			if !tt.valid {
				require.ErrorIs(t, err, snapshot.ErrInvalidDataSubset)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, subset.String())
		})
	}
}

func TestDataSubset_Select(t *testing.T) {
	t.Parallel()

	hashes := make([]string, 200)
	for i := range hashes {
		hashes[i] = fmt.Sprintf("%064x", i)
	}

	t.Run(`Given a percentage subset,
	when Select is called,
	then it returns that share of the objects, rounded up, without repeating any`, func(t *testing.T) {
		t.Parallel()

		subset, err := snapshot.ParseDataSubset("10%")
		require.NoError(t, err)

		selected := subset.Select(hashes, rand.New(rand.NewPCG(1, 2)))

		require.Len(t, selected, 20)
		require.Subset(t, hashes, selected)
		require.IsNonDecreasing(t, selected)
		tiny, err := snapshot.ParseDataSubset("0.1%")
		require.NoError(t, err)
		require.Len(t, tiny.Select(hashes, rand.New(rand.NewPCG(1, 2))), 1)
	})

	t.Run(`Given every part of a subset split in parts,
	when Select is called for each part,
	then every object is selected exactly once and always by the same part`, func(t *testing.T) {
		t.Parallel()

		seen := map[string]int{}
		for part := 1; part <= 3; part++ {
			subset, err := snapshot.ParseDataSubset(fmt.Sprintf("%d/3", part))
			require.NoError(t, err)
			selected := subset.Select(hashes, nil)
			require.Equal(t, selected, subset.Select(hashes, nil))
			require.NotEmpty(t, selected)
			for _, hash := range selected {
				seen[hash]++
			}
		}
		require.Len(t, seen, len(hashes))
		for _, count := range seen {
			require.Equal(t, 1, count)
		}
	})
}
