package repository_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestRewrap_Do(t *testing.T) {
	errTest := errors.New("test error")
	type args struct {
		repositoryPath    string
		oldPrivateKeyPath string
		newPublicKeyPath  string
	}
	tests := []struct {
		name string
		args args
		expectedErr, getConfigErr,
		readPrivateErr, decodeErr,
		ReadPublicErr, encryptErr,
		createErr, fingerPrintErr error
		config                     *repository.Config
		privateKey                 *rsa.PrivateKey
		publicKey                  *rsa.PublicKey
		symmetricKey, encryptedKey []byte
	}{
		{
			name: "and get config returns error, then it returns same error",
			args: args{
				repositoryPath:    "path",
				oldPrivateKeyPath: "old",
				newPublicKeyPath:  "new",
			},
			getConfigErr: errTest,
			expectedErr:  errTest,
		},
		{
			name: "and read private key returns error, then it returns same error",
			args: args{
				repositoryPath:    "path",
				oldPrivateKeyPath: "old",
				newPublicKeyPath:  "new",
			},
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("key", "finger", uint64(1)),
			}.Build(t)),
			readPrivateErr: errTest,
			expectedErr:    errTest,
		},
		{
			name: "and decode symmetric key returns error, then it returns same error",
			args: args{
				repositoryPath:    "path",
				oldPrivateKeyPath: "old",
				newPublicKeyPath:  "new",
			},
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("key", "finger", uint64(1)),
			}.Build(t)),
			privateKey:  &rsa.PrivateKey{},
			decodeErr:   errTest,
			expectedErr: errTest,
		},
		{
			name: "and read public key returns error, then it returns same error",
			args: args{
				repositoryPath:    "path",
				oldPrivateKeyPath: "old",
				newPublicKeyPath:  "new",
			},
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("key", "finger", uint64(1)),
			}.Build(t)),
			privateKey:    &rsa.PrivateKey{},
			symmetricKey:  []byte("key"),
			ReadPublicErr: errTest,
			expectedErr:   errTest,
		},
		{
			name: "and fingerPrint returns error, then it returns same error",
			args: args{
				repositoryPath:    "path",
				oldPrivateKeyPath: "old",
				newPublicKeyPath:  "new",
			},
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("key", "finger", uint64(1)),
			}.Build(t)),
			privateKey:     &rsa.PrivateKey{},
			symmetricKey:   []byte("key"),
			fingerPrintErr: errTest,
			expectedErr:    errTest,
		},
		{
			name: "and encrypt symmetric key returns error, then it returns same error",
			args: args{
				repositoryPath:    "path",
				oldPrivateKeyPath: "old",
				newPublicKeyPath:  "new",
			},
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("key", "finger", uint64(1)),
			}.Build(t)),
			privateKey:   &rsa.PrivateKey{},
			symmetricKey: []byte("key"),
			publicKey:    &rsa.PublicKey{},
			encryptErr:   errTest,
			expectedErr:  errTest,
		},
		{
			name: "and create config returns error, then it returns same error",
			args: args{
				repositoryPath:    "path",
				oldPrivateKeyPath: "old",
				newPublicKeyPath:  "new",
			},
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("key", "finger", uint64(1)),
			}.Build(t)),
			privateKey:   &rsa.PrivateKey{},
			symmetricKey: []byte("key"),
			publicKey:    &rsa.PublicKey{},
			encryptedKey: []byte("enc-key"),
			createErr:    errTest,
			expectedErr:  errTest,
		},
		{
			name: "with no error, then it returns nil",
			args: args{
				repositoryPath:    "path",
				oldPrivateKeyPath: "old",
				newPublicKeyPath:  "new",
			},
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("key", "finger", uint64(1)),
			}.Build(t)),
			privateKey:   &rsa.PrivateKey{},
			symmetricKey: []byte("key"),
			publicKey:    &rsa.PublicKey{},
			encryptedKey: []byte("enc-key"),
		},
	}
	for _, tt := range tests {
		t.Run(`Given a Rewrap service,
		when Do method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			folderRepo := &repository.FolderRepositoryMock{}
			folderRepo.GetConfigFunc = func(_ context.Context, _ string) (*repository.Config, error) {
				return tt.config, tt.getConfigErr
			}
			folderRepo.CreateConfigFunc = func(_ context.Context, _ string, _ *repository.Config) error {
				return tt.createErr
			}
			symmetricRepo := &encryption.SymmetricKeyRepositoryMock{}
			symmetricRepo.DecodeAndDecryptSymmetricKeyFunc = func(_ context.Context, _ *rsa.PrivateKey, _ string) ([]byte, error) {
				return tt.symmetricKey, tt.decodeErr
			}
			symmetricRepo.EncryptSymmetricKeyFunc = func(_ context.Context, _ *rsa.PublicKey, _ []byte) ([]byte, error) {
				return tt.encryptedKey, tt.encryptErr
			}
			asymmetricRepo := &encryption.AsymmetricKeyPairRepositoryMock{}
			asymmetricRepo.ReadRSAPrivateKeyFunc = func(_ context.Context, _ string) (*rsa.PrivateKey, error) {
				return tt.privateKey, tt.readPrivateErr
			}
			asymmetricRepo.ReadRSAPublicKeyFunc = func(_ context.Context, _ string) (*rsa.PublicKey, error) {
				return tt.publicKey, tt.ReadPublicErr
			}
			asymmetricRepo.PublicKeyFingerPrintFunc = func(_ context.Context, _ *rsa.PublicKey) (string, error) {
				return "fingerprint", tt.fingerPrintErr
			}
			svc := repository.NewRewrap(folderRepo, symmetricRepo, asymmetricRepo)
			err := svc.Do(t.Context(), tt.args.repositoryPath, tt.args.oldPrivateKeyPath, tt.args.newPublicKeyPath)
			if err != nil {
				require.ErrorAs(t, err, &tt.expectedErr)
			}
			if tt.expectedErr != nil {
				require.Error(t, err)
			}
		})
	}
}
