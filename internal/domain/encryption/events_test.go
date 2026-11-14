package encryption_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/stretchr/testify/require"
)

func TestEncryptionEvents(t *testing.T) {
	t.Parallel()

	t.Run(`Given an encryption config after a key rotation,
	when SymmetricKeyRotated is called,
	then the event carries the new generation and the rekeyed objects`, func(t *testing.T) {
		t.Parallel()

		conf := encryption.NewEncryptionConfig("key", "fingerprint", 4)

		rotated := conf.SymmetricKeyRotated("repository-id", 12)

		require.Equal(t, encryption.SymmetricKeyRotatedEventName, rotated.EventName())
		require.Equal(t, "repository-id", rotated.RepositoryID())
		require.Equal(t, uint64(4), rotated.Generation())
		require.Equal(t, 12, rotated.RekeyedObjects())
	})

	t.Run(`Given an encryption config after a rewrap,
	when KeyRewrapped is called,
	then the event carries the new public key fingerprint`, func(t *testing.T) {
		t.Parallel()

		conf := encryption.NewEncryptionConfig("key", "new-fingerprint", 1)

		rewrapped := conf.KeyRewrapped("repository-id")

		require.Equal(t, encryption.KeyRewrappedEventName, rewrapped.EventName())
		require.Equal(t, "repository-id", rewrapped.RepositoryID())
		require.Equal(t, "new-fingerprint", rewrapped.PublicKeyFingerprint())
	})
}
