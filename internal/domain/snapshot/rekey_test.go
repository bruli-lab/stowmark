package snapshot_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"io"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestReKey_Do(t *testing.T) {
	errTest := errors.New("test error")
	list := []string{"object1", "object2"}
	type args struct {
		repositoryPath string
		privateKeyPath string
		publicKeyPath  string
	}
	tests := []struct {
		name string
		args args
		expectedErr, getConfigErr,
		readPrivateKeyErr, decodeErr,
		readPublicKeyErr, generateErr,
		encryptErr, fingerPrintErr,
		listErr, readObjectErr,
		abortErr, saveObjectErr,
		createConfigErr, deleteErr error
		config     *repository.Config
		privateKey *rsa.PrivateKey
		publicKey  *rsa.PublicKey
		newSymmetricKey, oldSymmetricKey,
		encryptedKey []byte
		fingerprint string
		list        []string
		abortCalls  int
		reader      io.ReadCloser
	}{
		{
			name:         "and get config returns error, then it returns same error",
			getConfigErr: errTest,
			expectedErr:  errTest,
		},
		{
			name:        "with a config without encryption, then it returns a mission encryption config to encrypt error",
			config:      new(fixtures.ConfigBuilder{}.Build(t)),
			expectedErr: snapshot.ErrMissingEncryptionConfigToEncrypt,
		},
		{
			name: "when red rsa private key returns an error, then it returns same error",
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
			}.Build(t)),
			expectedErr:       errTest,
			readPrivateKeyErr: errTest,
		},
		{
			name: "when decode symmetric key returns an error, then it returns same error",
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
			}.Build(t)),
			privateKey:  &rsa.PrivateKey{},
			expectedErr: errTest,
			decodeErr:   errTest,
		},
		{
			name: "when read public key returns an error, then it returns same error",
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
			}.Build(t)),
			privateKey:       &rsa.PrivateKey{},
			oldSymmetricKey:  []byte("old-symmetric-key"),
			expectedErr:      errTest,
			readPublicKeyErr: errTest,
		},
		{
			name: "when generate new symmetric key returns an error, then it returns same error",
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
			}.Build(t)),
			privateKey:      &rsa.PrivateKey{},
			oldSymmetricKey: []byte("old-symmetric-key"),
			publicKey:       &rsa.PublicKey{},
			expectedErr:     errTest,
			generateErr:     errTest,
		},
		{
			name: "when encrypt new symmetric key returns an error, then it returns same error",
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
			}.Build(t)),
			privateKey:      &rsa.PrivateKey{},
			oldSymmetricKey: []byte("old-symmetric-key"),
			publicKey:       &rsa.PublicKey{},
			newSymmetricKey: []byte("new-symmetric-key"),
			expectedErr:     errTest,
			encryptErr:      errTest,
		},
		{
			name: "when get public key fingerprint returns an error, then it returns same error",
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
			}.Build(t)),
			privateKey:      &rsa.PrivateKey{},
			oldSymmetricKey: []byte("old-symmetric-key"),
			publicKey:       &rsa.PublicKey{},
			newSymmetricKey: []byte("new-symmetric-key"),
			encryptedKey:    []byte("encrypted-key"),
			expectedErr:     errTest,
			fingerPrintErr:  errTest,
		},
		{
			name: "when list objects returns an error, then it returns same error",
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
			}.Build(t)),
			privateKey:      &rsa.PrivateKey{},
			oldSymmetricKey: []byte("old-symmetric-key"),
			publicKey:       &rsa.PublicKey{},
			newSymmetricKey: []byte("new-symmetric-key"),
			encryptedKey:    []byte("encrypted-key"),
			fingerprint:     "fingerprint",
			expectedErr:     errTest,
			listErr:         errTest,
		},
		{
			name: "when read encrypt object returns an error, then it returns same error",
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
			}.Build(t)),
			privateKey:      &rsa.PrivateKey{},
			oldSymmetricKey: []byte("old-symmetric-key"),
			publicKey:       &rsa.PublicKey{},
			newSymmetricKey: []byte("new-symmetric-key"),
			encryptedKey:    []byte("encrypted-key"),
			fingerprint:     "fingerprint",
			list:            list,
			expectedErr:     errTest,
			readObjectErr:   errTest,
			abortCalls:      1,
		},
		{
			name: "when save rekey object returns an error, then it returns same error",
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
			}.Build(t)),
			privateKey:      &rsa.PrivateKey{},
			oldSymmetricKey: []byte("old-symmetric-key"),
			publicKey:       &rsa.PublicKey{},
			newSymmetricKey: []byte("new-symmetric-key"),
			encryptedKey:    []byte("encrypted-key"),
			fingerprint:     "fingerprint",
			list:            list,
			reader:          io.NopCloser(nil),
			expectedErr:     errTest,
			saveObjectErr:   errTest,
			abortCalls:      1,
		},
		{
			name: "when create config returns an error, then it returns same error",
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
			}.Build(t)),
			privateKey:      &rsa.PrivateKey{},
			oldSymmetricKey: []byte("old-symmetric-key"),
			publicKey:       &rsa.PublicKey{},
			newSymmetricKey: []byte("new-symmetric-key"),
			encryptedKey:    []byte("encrypted-key"),
			fingerprint:     "fingerprint",
			list:            list,
			reader:          io.NopCloser(nil),
			expectedErr:     errTest,
			createConfigErr: errTest,
			abortCalls:      1,
		},
		{
			name: "when create config returns an error and aborted key, then it returns errors",
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
			}.Build(t)),
			privateKey:      &rsa.PrivateKey{},
			oldSymmetricKey: []byte("old-symmetric-key"),
			publicKey:       &rsa.PublicKey{},
			newSymmetricKey: []byte("new-symmetric-key"),
			encryptedKey:    []byte("encrypted-key"),
			fingerprint:     "fingerprint",
			list:            list,
			reader:          io.NopCloser(nil),
			expectedErr:     errTest,
			createConfigErr: errTest,
			abortErr:        errTest,
			abortCalls:      1,
		},
		{
			name: "when delete encrypted generation returns an error, then it returns same error",
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
			}.Build(t)),
			privateKey:      &rsa.PrivateKey{},
			oldSymmetricKey: []byte("old-symmetric-key"),
			publicKey:       &rsa.PublicKey{},
			newSymmetricKey: []byte("new-symmetric-key"),
			encryptedKey:    []byte("encrypted-key"),
			fingerprint:     "fingerprint",
			list:            list,
			reader:          io.NopCloser(nil),
			expectedErr:     errTest,
			deleteErr:       errTest,
			abortCalls:      0,
		},
		{
			name: "with no errors, then it returns nil",
			config: new(fixtures.ConfigBuilder{
				EncryptionConfig: encryption.NewEncryptionConfig("abc", "def", uint64(1)),
			}.Build(t)),
			privateKey:      &rsa.PrivateKey{},
			oldSymmetricKey: []byte("old-symmetric-key"),
			publicKey:       &rsa.PublicKey{},
			newSymmetricKey: []byte("new-symmetric-key"),
			encryptedKey:    []byte("encrypted-key"),
			fingerprint:     "fingerprint",
			list:            list,
			reader:          io.NopCloser(nil),
			abortCalls:      0,
		},
	}
	for _, tt := range tests {
		t.Run(`Given ReKey service,
		when Do method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			folderRepo := &repository.FolderRepositoryMock{}
			folderRepo.GetConfigFunc = func(_ context.Context, _ string) (*repository.Config, error) {
				return tt.config, tt.getConfigErr
			}
			folderRepo.CreateConfigFunc = func(_ context.Context, _ string, _ *repository.Config) error {
				return tt.createConfigErr
			}

			symmetricRepo := &encryption.SymmetricKeyRepositoryMock{}
			symmetricRepo.DecodeAndDecryptSymmetricKeyFunc = func(_ context.Context, _ *rsa.PrivateKey, _ string) ([]byte, error) {
				return tt.oldSymmetricKey, tt.decodeErr
			}
			symmetricRepo.GenerateSymmetricKeyFunc = func(_ context.Context) ([]byte, error) {
				return tt.newSymmetricKey, tt.generateErr
			}
			symmetricRepo.EncryptSymmetricKeyFunc = func(_ context.Context, _ *rsa.PublicKey, _ []byte) ([]byte, error) {
				return tt.encryptedKey, tt.encryptErr
			}

			asymmetricRepo := &encryption.AsymmetricKeyPairRepositoryMock{}
			asymmetricRepo.ReadRSAPrivateKeyFunc = func(_ context.Context, _ string) (*rsa.PrivateKey, error) {
				return tt.privateKey, tt.readPrivateKeyErr
			}
			asymmetricRepo.ReadRSAPublicKeyFunc = func(_ context.Context, _ string) (*rsa.PublicKey, error) {
				return tt.publicKey, tt.readPublicKeyErr
			}
			asymmetricRepo.PublicKeyFingerPrintFunc = func(_ context.Context, _ *rsa.PublicKey) (string, error) {
				return tt.fingerprint, tt.fingerPrintErr
			}

			objectRepo := &snapshot.ObjectRepositoryMock{}
			objectRepo.ListEncryptedObjectsFunc = func(_ context.Context, _ uint64) ([]string, error) {
				return tt.list, tt.listErr
			}
			objectRepo.ReadEncryptedObjectFunc = func(_ context.Context, _ string, _ uint64, _ []byte) (io.ReadCloser, error) {
				return tt.reader, tt.readObjectErr
			}
			objectRepo.AbortRekeyFunc = func(_ context.Context, _ uint64) error {
				return tt.abortErr
			}
			objectRepo.SaveRekeyedObjectFunc = func(_ context.Context, _ string, _ io.Reader, _ uint64, _ []byte) error {
				return tt.saveObjectErr
			}
			objectRepo.DeleteEncryptedGenerationFunc = func(_ context.Context, _ uint64) error {
				return tt.deleteErr
			}
			objectRepo.AbortRekeyFunc = func(_ context.Context, _ uint64) error {
				return tt.abortErr
			}

			svc := snapshot.NewReKey(folderRepo, symmetricRepo, asymmetricRepo, objectRepo)
			err := svc.Do(t.Context(), tt.args.repositoryPath, tt.args.privateKeyPath, tt.args.publicKeyPath)
			require.Len(t, objectRepo.AbortRekeyCalls(), tt.abortCalls)
			if err != nil {
				require.ErrorAs(t, err, &tt.expectedErr)
				return
			}
			if tt.expectedErr != nil {
				require.Error(t, err)
			}
		})
	}
}
