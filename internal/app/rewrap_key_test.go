package app_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark/internal/app"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestRewrapKey_Handle(t *testing.T) {
	t.Parallel()
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
			name: "and the repository is not encrypted, then it returns a repository not encrypted error",
			args: args{
				repositoryPath:    "path",
				oldPrivateKeyPath: "old",
				newPublicKeyPath:  "new",
			},
			config:      new(fixtures.ConfigBuilder{}.Build(t)),
			expectedErr: app.ErrRepositoryNotEncrypted,
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
		t.Run(`Given a RewrapKey command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			folderRepo := &app.FolderRepositoryMock{}
			folderRepo.GetConfigFunc = func(_ context.Context, _ string) (*repository.Config, error) {
				return tt.config, tt.getConfigErr
			}
			folderRepo.CreateConfigFunc = func(_ context.Context, _ string, _ *repository.Config) error {
				return tt.createErr
			}
			symmetricRepo := &app.SymmetricKeyRepositoryMock{}
			symmetricRepo.DecodeAndDecryptSymmetricKeyFunc = func(_ context.Context, _ *rsa.PrivateKey, _ string) ([]byte, error) {
				return tt.symmetricKey, tt.decodeErr
			}
			symmetricRepo.EncryptSymmetricKeyFunc = func(_ context.Context, _ *rsa.PublicKey, _ []byte) ([]byte, error) {
				return tt.encryptedKey, tt.encryptErr
			}
			asymmetricRepo := &app.AsymmetricKeyPairRepositoryMock{}
			asymmetricRepo.ReadRSAPrivateKeyFunc = func(_ context.Context, _ string) (*rsa.PrivateKey, error) {
				return tt.privateKey, tt.readPrivateErr
			}
			asymmetricRepo.ReadRSAPublicKeyFunc = func(_ context.Context, _ string) (*rsa.PublicKey, error) {
				return tt.publicKey, tt.ReadPublicErr
			}
			asymmetricRepo.PublicKeyFingerPrintFunc = func(_ context.Context, _ *rsa.PublicKey) (string, error) {
				return "fingerprint", tt.fingerPrintErr
			}
			handler := app.NewRewrapKey(folderRepo, asymmetricRepo, symmetricRepo, newUnlockedLocker())
			events, err := handler.Handle(t.Context(), app.RewrapKeyCommand{
				RepositoryPath: tt.args.repositoryPath,
				OldPrivateKey:  tt.args.oldPrivateKeyPath,
				NewPublicKey:   tt.args.newPublicKeyPath,
			})
			require.Empty(t, events)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
