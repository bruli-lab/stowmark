package app_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"math/big"
	"testing"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/stretchr/testify/require"
)

func TestGenerateKey_Handle(t *testing.T) {
	t.Parallel()

	privateKey := &rsa.PrivateKey{N: big.NewInt(77), E: 3}
	errRepo := errors.New("repository failed")
	tests := []struct {
		name           string
		generateErr    error
		privateErr     error
		publicErr      error
		expectedErr    error
		expectedWrites int
	}{
		{name: "then it saves the private and the public key", expectedWrites: 2},
		{name: "and the key cannot be generated, then it returns the error without saving anything", generateErr: errRepo, expectedErr: errRepo},
		{name: "and the private key cannot be saved, then it returns the error", privateErr: errRepo, expectedErr: errRepo, expectedWrites: 1},
		{name: "and the public key cannot be saved, then it returns the error", publicErr: errRepo, expectedErr: errRepo, expectedWrites: 2},
	}
	for _, tt := range tests {
		t.Run(`Given a GenerateKey command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			generator := &app.KeyGeneratorMock{
				GenerateRSAKeyFunc: func(context.Context) (*rsa.PrivateKey, error) { return privateKey, tt.generateErr },
			}
			asymmetric := &app.AsymmetricKeyPairRepositoryMock{
				CreatePrivateKeyFunc: func(context.Context, string, *rsa.PrivateKey) error { return tt.privateErr },
				CreatePublicKeyFunc:  func(context.Context, string, *rsa.PublicKey) error { return tt.publicErr },
			}
			keys, err := encryption.NewAsymmetricKeyPair("keys")
			require.NoError(t, err)

			events, err := app.NewGenerateKey(generator, asymmetric).Handle(t.Context(), app.GenerateKeyCommand{Keys: keys})

			require.Empty(t, events)
			require.Equal(t, tt.expectedWrites, len(asymmetric.CreatePrivateKeyCalls())+len(asymmetric.CreatePublicKeyCalls()))
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, keys.PrivateKey(), asymmetric.CreatePrivateKeyCalls()[0].FilePath)
			require.Same(t, privateKey, asymmetric.CreatePrivateKeyCalls()[0].PrivateKey)
			require.Equal(t, keys.PublicKey(), asymmetric.CreatePublicKeyCalls()[0].FilePath)
			require.Same(t, &privateKey.PublicKey, asymmetric.CreatePublicKeyCalls()[0].PublicKey)
		})
	}
}
