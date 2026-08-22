package encryption_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/stretchr/testify/require"
)

func TestCreateAsymmetricKeyPair_Create(t *testing.T) {
	errTest := errors.New("test error")
	type args struct {
		keyPairs *encryption.AsymmetricKeyPair
	}
	keys, err := encryption.NewAsymmetricKeyPair("test")
	require.NoError(t, err)
	tests := []struct {
		name string
		args args
		expectedErr, privateErr,
		publicErr error
	}{
		{
			name:        "and create private key returns an error, then it returns same error",
			args:        args{keyPairs: keys},
			privateErr:  errTest,
			expectedErr: errTest,
		},
		{
			name:        "and create pubic key returns an error, then it returns same error",
			args:        args{keyPairs: keys},
			privateErr:  errTest,
			expectedErr: errTest,
		},
		{
			name: "with no errors, then it returns nil",
			args: args{keyPairs: keys},
		},
	}
	for _, tt := range tests {
		t.Run(`Given a CreateAsymmetricKeyPair strut,
		when Create method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &encryption.AsymmetricKeyPairRepositoryMock{}
			repo.CreatePrivateKeyFunc = func(_ context.Context, _ string, _ *rsa.PrivateKey) error {
				return tt.privateErr
			}
			repo.CreatePublicKeyFunc = func(_ context.Context, _ string, _ *rsa.PublicKey) error {
				return tt.publicErr
			}
			svc := encryption.NewCreateAsymmetricKeyPair(repo)
			err := svc.Create(t.Context(), tt.args.keyPairs)
			if err != nil {
				require.ErrorAs(t, err, &tt.expectedErr)
			}
			if tt.expectedErr != nil {
				require.Error(t, err)
			}
		})
	}
}
