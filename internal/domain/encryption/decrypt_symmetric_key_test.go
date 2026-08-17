package encryption_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/stretchr/testify/require"
)

func TestDecryptSymmetricKey_Decrypt(t *testing.T) {
	errTest := errors.New("test error")
	type args struct {
		privateKeyPath string
		encodedKey     string
	}
	tests := []struct {
		name             string
		args             args
		expectedKey, key []byte
		expectedErr, readRSAErr,
		decodeErr error
		privateKey *rsa.PrivateKey
	}{
		{
			name:        "and ReadRSAPrivateKey returns error, then it returns same error",
			readRSAErr:  errTest,
			expectedErr: errTest,
		},
		{
			name:        "and DecodeAndDecryptSymmetricKey returns error, then it returns same error",
			privateKey:  &rsa.PrivateKey{},
			decodeErr:   errTest,
			expectedErr: errTest,
		},
		{
			name:        "without errors, then it returns the symmetric key",
			privateKey:  &rsa.PrivateKey{},
			key:         []byte("key"),
			expectedKey: []byte("key"),
		},
	}
	for _, tt := range tests {
		t.Run(`Given a DecryptSymmetricKey service,
		when Decrypt method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			symmetricRepo := &encryption.SymmetricKeyRepositoryMock{}
			symmetricRepo.DecodeAndDecryptSymmetricKeyFunc = func(_ context.Context, _ *rsa.PrivateKey, _ string) ([]byte, error) {
				return tt.key, tt.decodeErr
			}
			asymmetricRepo := &encryption.AsymmetricKeyPairRepositoryMock{}
			asymmetricRepo.ReadRSAPrivateKeyFunc = func(_ context.Context, _ string) (*rsa.PrivateKey, error) {
				return tt.privateKey, tt.readRSAErr
			}
			svc := encryption.NewDecryptSymmetricKey(symmetricRepo, asymmetricRepo)
			got, err := svc.Decrypt(t.Context(), tt.args.privateKeyPath, tt.args.encodedKey)
			if err != nil {
				require.ErrorAs(t, err, &tt.expectedErr)
				return
			}
			require.Equal(t, tt.expectedKey, got)
			got2, err := svc.Decrypt(t.Context(), tt.args.privateKeyPath, tt.args.encodedKey)
			require.NoError(t, err)
			require.Equal(t, got, got2)
		})
	}
}
