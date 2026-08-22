package encryption

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
)

const rsaKeySize = 3072

type CreateAsymmetricKeyPair struct {
	repo AsymmetricKeyPairRepository
}

func (c CreateAsymmetricKeyPair) Create(ctx context.Context, keyPairs *AsymmetricKeyPair) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}
	if err := privateKey.Validate(); err != nil {
		return fmt.Errorf("private key is invalid: %w", err)
	}
	if err := c.repo.CreatePrivateKey(ctx, keyPairs.privateKey, privateKey); err != nil {
		return err
	}
	return c.repo.CreatePublicKey(ctx, keyPairs.publicKey, &privateKey.PublicKey)
}

func NewCreateAsymmetricKeyPair(repo AsymmetricKeyPairRepository) *CreateAsymmetricKeyPair {
	return &CreateAsymmetricKeyPair{repo: repo}
}
