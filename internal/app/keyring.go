package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
)

var (
	ErrMissingEncryptionConfigToEncrypt = errors.New("missing encryption config to encrypt")
	ErrRepositoryNotEncrypted           = errors.New("repository is not encrypted")
)

type objectKey struct {
	symmetricKey []byte
	generation   uint64
}

type wrappedKey struct {
	encodedKey  string
	fingerprint string
}

type keyring struct {
	folderRepo     FolderRepository
	asymmetricRepo AsymmetricKeyPairRepository
	symmetricRepo  SymmetricKeyRepository
}

func (k keyring) forRepository(ctx context.Context, repositoryPath string, privateKeyPath *string) (objectKey, error) {
	if privateKeyPath == nil {
		return objectKey{}, nil
	}
	conf, err := k.folderRepo.GetConfig(ctx, repositoryPath)
	if err != nil {
		return objectKey{}, err
	}
	return k.forConfig(ctx, conf.Encryption(), privateKeyPath)
}

func (k keyring) forConfig(ctx context.Context, conf *encryption.EncryptionConfig, privateKeyPath *string) (objectKey, error) {
	if privateKeyPath == nil {
		return objectKey{}, nil
	}
	if conf == nil {
		return objectKey{}, ErrMissingEncryptionConfigToEncrypt
	}
	symmetricKey, err := k.unwrap(ctx, *privateKeyPath, conf)
	if err != nil {
		return objectKey{}, err
	}
	return objectKey{symmetricKey: symmetricKey, generation: conf.Generation()}, nil
}

func (k keyring) unwrap(ctx context.Context, privateKeyPath string, conf *encryption.EncryptionConfig) ([]byte, error) {
	privateKey, err := k.asymmetricRepo.ReadRSAPrivateKey(ctx, privateKeyPath)
	if err != nil {
		return nil, err
	}
	return k.symmetricRepo.DecodeAndDecryptSymmetricKey(ctx, privateKey, conf.EncryptedKey())
}

func (k keyring) wrap(ctx context.Context, publicKeyPath string, symmetricKey []byte) (wrappedKey, error) {
	publicKey, err := k.asymmetricRepo.ReadRSAPublicKey(ctx, publicKeyPath)
	if err != nil {
		return wrappedKey{}, err
	}
	encryptedKey, err := k.symmetricRepo.EncryptSymmetricKey(ctx, publicKey, symmetricKey)
	if err != nil {
		return wrappedKey{}, fmt.Errorf("encrypt symmetric key: %w", err)
	}
	fingerprint, err := k.asymmetricRepo.PublicKeyFingerPrint(ctx, publicKey)
	if err != nil {
		return wrappedKey{}, err
	}
	return wrappedKey{encodedKey: base64.StdEncoding.EncodeToString(encryptedKey), fingerprint: fingerprint}, nil
}

func (k keyring) newEncryptionConfig(ctx context.Context, publicKeyPath string) (*encryption.EncryptionConfig, error) {
	symmetricKey, err := k.symmetricRepo.GenerateSymmetricKey(ctx)
	if err != nil {
		return nil, err
	}
	defer clear(symmetricKey)
	wrapped, err := k.wrap(ctx, publicKeyPath, symmetricKey)
	if err != nil {
		return nil, err
	}
	return encryption.NewEncryptionConfig(wrapped.encodedKey, wrapped.fingerprint, uint64(1)), nil
}

func newKeyring(folderRepo FolderRepository, asymmetricRepo AsymmetricKeyPairRepository, symmetricRepo SymmetricKeyRepository) keyring {
	return keyring{folderRepo: folderRepo, asymmetricRepo: asymmetricRepo, symmetricRepo: symmetricRepo}
}
