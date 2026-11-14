package app

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

var (
	ErrMissingEncryptionConfigToEncrypt = errors.New("missing encryption config to encrypt")
	ErrRepositoryNotEncrypted           = errors.New("repository is not encrypted")
	ErrPrivateKeyRequired               = errors.New("the private key is required to read an encrypted format version 3 repository")
	ErrMissingMetadataKey               = errors.New("the encryption configuration of a format version 3 repository has no metadata key")
)

type wrappedKey struct {
	encodedKey  string
	fingerprint string
}

type repositoryKeys struct {
	data     []byte
	metadata []byte
}

func (r repositoryKeys) clear() {
	clear(r.data)
	clear(r.metadata)
}

type keyring struct {
	folderRepo     FolderRepository
	asymmetricRepo AsymmetricKeyPairRepository
	symmetricRepo  SymmetricKeyRepository
}

func (k keyring) forRepository(ctx context.Context, repositoryPath string, privateKeyPath *string) (repository.Codec, error) {
	conf, err := k.folderRepo.GetConfig(ctx, repositoryPath)
	if err != nil {
		return repository.Codec{}, err
	}
	return k.forConfig(ctx, conf, privateKeyPath)
}

func (k keyring) forConfig(ctx context.Context, conf *repository.Config, privateKeyPath *string) (repository.Codec, error) {
	codec := conf.Codec()
	encryptionConf := conf.Encryption()
	if privateKeyPath == nil {
		if encryptionConf != nil && conf.FormatVersion().ContentAddressed() {
			return repository.Codec{}, ErrPrivateKeyRequired
		}
		return codec, nil
	}
	if encryptionConf == nil {
		return repository.Codec{}, ErrMissingEncryptionConfigToEncrypt
	}
	keys, err := k.unwrap(ctx, *privateKeyPath, encryptionConf)
	if err != nil {
		return repository.Codec{}, err
	}
	defer keys.clear()
	codec = codec.WithDataKey(keys.data, encryptionConf.Generation())
	if !conf.FormatVersion().ContentAddressed() {
		return codec, nil
	}
	if keys.metadata == nil {
		return repository.Codec{}, ErrMissingMetadataKey
	}
	return codec.WithMetadataKey(keys.metadata), nil
}

func (k keyring) unwrap(ctx context.Context, privateKeyPath string, conf *encryption.EncryptionConfig) (repositoryKeys, error) {
	privateKey, err := k.asymmetricRepo.ReadRSAPrivateKey(ctx, privateKeyPath)
	if err != nil {
		return repositoryKeys{}, err
	}
	data, err := k.symmetricRepo.DecodeAndDecryptSymmetricKey(ctx, privateKey, conf.EncryptedKey())
	if err != nil {
		return repositoryKeys{}, err
	}
	if !conf.HasMetadataKey() {
		return repositoryKeys{data: data}, nil
	}
	metadata, err := k.symmetricRepo.DecodeAndDecryptSymmetricKey(ctx, privateKey, conf.EncryptedMetadataKey())
	if err != nil {
		clear(data)
		return repositoryKeys{}, fmt.Errorf("decrypt metadata key: %w", err)
	}
	return repositoryKeys{data: data, metadata: metadata}, nil
}

func (k keyring) wrap(ctx context.Context, publicKeyPath string, symmetricKey []byte) (wrappedKey, error) {
	publicKey, err := k.asymmetricRepo.ReadRSAPublicKey(ctx, publicKeyPath)
	if err != nil {
		return wrappedKey{}, err
	}
	return k.wrapWith(ctx, publicKey, symmetricKey)
}

func (k keyring) wrapWith(ctx context.Context, publicKey *rsa.PublicKey, symmetricKey []byte) (wrappedKey, error) {
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

func (k keyring) rewrapMetadataKey(ctx context.Context, publicKeyPath string, metadataKey []byte, conf *encryption.EncryptionConfig) error {
	if metadataKey == nil {
		return nil
	}
	wrapped, err := k.wrap(ctx, publicKeyPath, metadataKey)
	if err != nil {
		return fmt.Errorf("wrap metadata key: %w", err)
	}
	conf.ChangeEncryptedMetadataKey(wrapped.encodedKey)
	return nil
}

func (k keyring) addMetadataKey(ctx context.Context, publicKeyPath string, conf *encryption.EncryptionConfig) error {
	metadataKey, err := k.symmetricRepo.GenerateSymmetricKey(ctx)
	if err != nil {
		return err
	}
	defer clear(metadataKey)
	return k.rewrapMetadataKey(ctx, publicKeyPath, metadataKey, conf)
}

func (k keyring) newEncryptionConfig(ctx context.Context, publicKeyPath string, version repository.FormatVersion) (*encryption.EncryptionConfig, error) {
	symmetricKey, err := k.symmetricRepo.GenerateSymmetricKey(ctx)
	if err != nil {
		return nil, err
	}
	defer clear(symmetricKey)
	wrapped, err := k.wrap(ctx, publicKeyPath, symmetricKey)
	if err != nil {
		return nil, err
	}
	conf := encryption.NewEncryptionConfig(wrapped.encodedKey, wrapped.fingerprint, uint64(1))
	if !version.ContentAddressed() {
		return conf, nil
	}
	if err := k.addMetadataKey(ctx, publicKeyPath, conf); err != nil {
		return nil, err
	}
	return conf, nil
}

func newKeyring(folderRepo FolderRepository, asymmetricRepo AsymmetricKeyPairRepository, symmetricRepo SymmetricKeyRepository) keyring {
	return keyring{folderRepo: folderRepo, asymmetricRepo: asymmetricRepo, symmetricRepo: symmetricRepo}
}
