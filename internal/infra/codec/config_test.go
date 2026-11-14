package codec_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/bruli-lab/stowmark/internal/infra/codec"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/stretchr/testify/require"
)

func TestMarshalConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		encryption *encryption.EncryptionConfig
	}{
		{
			name: "without encryption, then it returns a config that builds back into the same domain config",
		},
		{
			name:       "with encryption, then it returns a config that builds back into the same domain config",
			encryption: encryption.NewEncryptionConfig("encrypted", "fingerprint", 4),
		},
	}

	for _, tt := range tests {
		t.Run(`Given a domain config,
		when MarshalConfig function is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			original := fixtures.ConfigBuilder{EncryptionConfig: tt.encryption}.Build(t)

			data, err := codec.MarshalConfig(context.Background(), &original)

			require.NoError(t, err)
			require.Equal(t, byte('\n'), data[len(data)-1])
			var conf model.Config
			require.NoError(t, json.Unmarshal(data, &conf))
			got, err := model.BuildConfigDomain(conf)
			require.NoError(t, err)
			require.Equal(t, original.ID(), got.ID())
			require.Equal(t, original.FormatVersion(), got.FormatVersion())
			require.True(t, original.CreatedAt().Truncate(1e9).Equal(got.CreatedAt()))
			require.Equal(t, original.Compression(), got.Compression())
			require.Equal(t, original.Encryption(), got.Encryption())
		})
	}

	t.Run(`Given a domain config without encryption,
	when MarshalConfig function is called,
	then the JSON does not contain an encryption field`, func(t *testing.T) {
		t.Parallel()

		original := fixtures.ConfigBuilder{}.Build(t)

		data, err := codec.MarshalConfig(context.Background(), &original)

		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &fields))
		require.NotContains(t, fields, "encryption")
	})

	t.Run(`Given a canceled context,
	when MarshalConfig function is called,
	then it returns a context canceled error`, func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		original := fixtures.ConfigBuilder{}.Build(t)

		_, err := codec.MarshalConfig(ctx, &original)

		require.ErrorIs(t, err, context.Canceled)
	})
}
