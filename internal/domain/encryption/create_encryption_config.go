package encryption

import (
	"context"
	"encoding/base64"
	"fmt"
)

type CreateEncryptionConfig struct {
	asymmetricRepo AsymmetricKeyPairRepository
	symmetricRepo  SymmetricKeyRepository
}

func (c CreateEncryptionConfig) Create(ctx context.Context, publicKeyPath string) (*EncryptionConfig, error) {
	publicKey, err := c.asymmetricRepo.ReadRSAPublicKey(ctx, publicKeyPath)
	if err != nil {
		return nil, err
	}
	symmetricKey, err := c.symmetricRepo.GenerateSymmetricKey(ctx)
	if err != nil {
		return nil, err
	}
	defer clear(symmetricKey)
	encryptedKey, err := c.symmetricRepo.EncryptSymmetricKey(ctx, publicKey, symmetricKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt symmetric key: %w", err)
	}
	fingerprint, err := c.asymmetricRepo.PublicKeyFingerPrint(ctx, publicKey)
	if err != nil {
		return nil, err
	}
	return NewEncryptionConfig(base64.StdEncoding.EncodeToString(encryptedKey), fingerprint, uint64(1)), nil
}

func NewCreateEncryptionConfig(asymmetricRepo AsymmetricKeyPairRepository, symmetricRepo SymmetricKeyRepository) *CreateEncryptionConfig {
	return &CreateEncryptionConfig{asymmetricRepo: asymmetricRepo, symmetricRepo: symmetricRepo}
}
