package repository_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestConfig_OrphanGenerations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		encryption *encryption.EncryptionConfig
		stored     []uint64
		expected   []uint64
	}{
		{
			name:       "of an encrypted repository, then every generation but the current one is an orphan",
			encryption: encryption.NewEncryptionConfig("key", "fingerprint", 2),
			stored:     []uint64{1, 2, 3},
			expected:   []uint64{1, 3},
		},
		{
			name:       "of an encrypted repository with only the current generation, then there are no orphans",
			encryption: encryption.NewEncryptionConfig("key", "fingerprint", 2),
			stored:     []uint64{2},
			expected:   []uint64{},
		},
		{
			name:     "of a plain repository, then every encrypted generation is an orphan",
			stored:   []uint64{1, 2},
			expected: []uint64{1, 2},
		},
	}
	for _, tt := range tests {
		t.Run(`Given the stored encrypted generations,
		when OrphanGenerations is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			config := fixtures.ConfigBuilder{EncryptionConfig: tt.encryption}.Build(t)

			require.Equal(t, tt.expected, config.OrphanGenerations(tt.stored))
		})
	}
}
