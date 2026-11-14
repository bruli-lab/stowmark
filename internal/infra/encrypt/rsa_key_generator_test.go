package encrypt_test

import (
	"context"
	"testing"

	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/stretchr/testify/require"
)

func TestRSAKeyGenerator_GenerateRSAKey(t *testing.T) {
	t.Parallel()

	t.Run(`Given an RSA key generator,
	when GenerateRSAKey is called,
	then it returns a valid key of the expected size`, func(t *testing.T) {
		t.Parallel()

		key, err := encrypt.NewRSAKeyGenerator().GenerateRSAKey(context.Background())

		require.NoError(t, err)
		require.NoError(t, key.Validate())
		require.Equal(t, encrypt.RSAKeySize, key.N.BitLen())
	})

	t.Run(`Given an RSA key generator and a cancelled context,
	when GenerateRSAKey is called,
	then it returns the context error`, func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := encrypt.NewRSAKeyGenerator().GenerateRSAKey(ctx)

		require.ErrorIs(t, err, context.Canceled)
	})
}
