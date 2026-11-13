package encryption_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/stretchr/testify/require"
)

func TestHydrateEncryptionConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		encryptionType string
		keyEncryption  string
		expectedErr    error
	}{
		{
			name:           "with the supported algorithms, then it returns the encryption config",
			encryptionType: encryption.DefaultEncryptionType,
			keyEncryption:  encryption.DefaultKeyEncryption,
		},
		{
			name:           "with an unknown encryption type, then it returns an unsupported encryption error",
			encryptionType: "chacha20-poly1305",
			keyEncryption:  encryption.DefaultKeyEncryption,
			expectedErr:    encryption.ErrUnsupportedEncryption,
		},
		{
			name:           "with an unknown key encryption, then it returns an unsupported encryption error",
			encryptionType: encryption.DefaultEncryptionType,
			keyEncryption:  "rsa-pkcs1v15",
			expectedErr:    encryption.ErrUnsupportedEncryption,
		},
		{
			name:        "without algorithms, then it returns an unsupported encryption error",
			expectedErr: encryption.ErrUnsupportedEncryption,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a HydrateEncryptionConfig function,
		when it is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := encryption.HydrateEncryptionConfig(tt.encryptionType, tt.keyEncryption, "key", "fingerprint", 3)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.encryptionType, got.EncryptionType())
			require.Equal(t, tt.keyEncryption, got.KeyEncryption())
			require.Equal(t, "key", got.EncryptedKey())
			require.Equal(t, "fingerprint", got.PublicKeyFingerprint())
			require.Equal(t, uint64(3), got.Generation())
		})
	}
}
