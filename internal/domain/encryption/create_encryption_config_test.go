package encryption_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/stretchr/testify/require"
)

func TestCreateEncryptionConfig_Create(t *testing.T) {
	errTest := errors.New("test error")
	type args struct {
		publicKeyPath string
	}
	publicKey := rsa.PublicKey{}
	tests := []struct {
		name string
		args args
		expectedErr, readRSAErr,
		generateErr, encryptErr,
		fingerPrintErr error
		publicKey *rsa.PublicKey
	}{
		{
			name:        "and read RSA public key returns error, then it returns same error",
			readRSAErr:  errTest,
			expectedErr: errTest,
		},
		{
			name:        "and generate symmetric key returns error, then it returns same error",
			publicKey:   &publicKey,
			generateErr: errTest,
			expectedErr: errTest,
		},
		{
			name:        "and encrypt symmetric key returns error, then it returns same error",
			publicKey:   &publicKey,
			encryptErr:  errTest,
			expectedErr: errTest,
		},
		{
			name:           "and public key finger print returns error, then it returns same error",
			publicKey:      &publicKey,
			fingerPrintErr: errTest,
			expectedErr:    errTest,
		},
		{
			name:      "with no errors, then it returns a valid encryption config",
			publicKey: &publicKey,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a CreateEncryptionConfig service,
		when Create method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			asymmetricKeyPairRepo := &encryption.AsymmetricKeyPairRepositoryMock{}
			asymmetricKeyPairRepo.ReadRSAPublicKeyFunc = func(_ context.Context, _ string) (*rsa.PublicKey, error) {
				return tt.publicKey, tt.readRSAErr
			}
			asymmetricKeyPairRepo.PublicKeyFingerPrintFunc = func(_ context.Context, _ *rsa.PublicKey) (string, error) {
				return "fingerprint", tt.fingerPrintErr
			}
			symmetricRepo := &encryption.SymmetricKeyRepositoryMock{}
			symmetricRepo.GenerateSymmetricKeyFunc = func(_ context.Context) ([]byte, error) {
				return []byte("bcd"), tt.generateErr
			}
			symmetricRepo.EncryptSymmetricKeyFunc = func(_ context.Context, _ *rsa.PublicKey, _ []byte) ([]byte, error) {
				return []byte("abc"), tt.encryptErr
			}
			svc := encryption.NewCreateEncryptionConfig(asymmetricKeyPairRepo, symmetricRepo)
			config, err := svc.Create(t.Context(), tt.args.publicKeyPath)
			if err != nil {
				require.ErrorAs(t, err, &tt.expectedErr)
				return
			}
			if tt.expectedErr != nil {
				require.Error(t, err)
			}
			require.NotNil(t, config)
			require.NotEmpty(t, config.EncryptedKey())
			require.NotEmpty(t, config.PublicKeyFingerprint())
		})
	}
}
