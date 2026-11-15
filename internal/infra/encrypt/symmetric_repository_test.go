package encrypt_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/stretchr/testify/require"
)

func newPrivateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	return privateKey
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestSymmetricRepository_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run(`Given a SymmetricRepository and an RSA key pair,
	when a symmetric key is generated, encrypted, encoded and decrypted,
	then it returns the generated key`, func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repo := encrypt.NewSymmetricRepository()
		privateKey := newPrivateKey(t)

		symmetricKey, err := repo.GenerateSymmetricKey(ctx)
		require.NoError(t, err)
		require.Len(t, symmetricKey, encrypt.AES256KeySize)
		encrypted, err := repo.EncryptSymmetricKey(ctx, &privateKey.PublicKey, symmetricKey)
		require.NoError(t, err)

		decrypted, err := repo.DecodeAndDecryptSymmetricKey(ctx, privateKey, base64.StdEncoding.EncodeToString(encrypted))

		require.NoError(t, err)
		require.Equal(t, symmetricKey, decrypted)
	})
}

func TestSymmetricRepository_DecodeAndDecryptSymmetricKey(t *testing.T) {
	t.Parallel()

	privateKey := newPrivateKey(t)
	otherKey := newPrivateKey(t)
	encrypt16 := func() string {
		encrypted, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &privateKey.PublicKey, make([]byte, 16), nil)
		require.NoError(t, err)
		return base64.StdEncoding.EncodeToString(encrypted)
	}
	valid := func() string {
		encrypted, err := encrypt.NewSymmetricRepository().EncryptSymmetricKey(context.Background(), &privateKey.PublicKey, make([]byte, encrypt.AES256KeySize))
		require.NoError(t, err)
		return base64.StdEncoding.EncodeToString(encrypted)
	}

	tests := []struct {
		name        string
		ctx         context.Context
		privateKey  *rsa.PrivateKey
		encodedKey  string
		expectedErr error
	}{
		{
			name:        "with a canceled context, then it returns a context canceled error",
			ctx:         canceledContext(),
			privateKey:  privateKey,
			encodedKey:  valid(),
			expectedErr: context.Canceled,
		},
		{
			name:        "without a private key, then it returns a private key required error",
			ctx:         context.Background(),
			encodedKey:  valid(),
			expectedErr: encrypt.ErrPrivateKeyRequired,
		},
		{
			name:        "with a key that is not base64, then it returns a base64 error",
			ctx:         context.Background(),
			privateKey:  privateKey,
			encodedKey:  "not base64!",
			expectedErr: base64.CorruptInputError(3),
		},
		{
			name:        "with a key encrypted for another private key, then it returns a decryption error",
			ctx:         context.Background(),
			privateKey:  otherKey,
			encodedKey:  valid(),
			expectedErr: rsa.ErrDecryption,
		},
		{
			name:        "with a key that is not 32 bytes long, then it returns an invalid key size error",
			ctx:         context.Background(),
			privateKey:  privateKey,
			encodedKey:  encrypt16(),
			expectedErr: encrypt.ErrInvalidKeySize,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a SymmetricRepository,
		when DecodeAndDecryptSymmetricKey method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := encrypt.NewSymmetricRepository().DecodeAndDecryptSymmetricKey(tt.ctx, tt.privateKey, tt.encodedKey)

			require.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestSymmetricRepository_CanceledContext(t *testing.T) {
	t.Parallel()

	t.Run(`Given a SymmetricRepository and a canceled context,
	when GenerateSymmetricKey and EncryptSymmetricKey methods are called,
	then they return a context canceled error`, func(t *testing.T) {
		t.Parallel()

		repo := encrypt.NewSymmetricRepository()

		_, err := repo.GenerateSymmetricKey(canceledContext())
		require.ErrorIs(t, err, context.Canceled)

		_, err = repo.EncryptSymmetricKey(canceledContext(), &newPrivateKey(t).PublicKey, make([]byte, encrypt.AES256KeySize))
		require.ErrorIs(t, err, context.Canceled)
	})
}
