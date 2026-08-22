package encrypt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const AES256KeySize = 32

type SymmetricRepository struct{}

func (s SymmetricRepository) DecodeAndDecryptSymmetricKey(ctx context.Context, privateKey *rsa.PrivateKey, encodedKey string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if privateKey == nil {
		return nil, fmt.Errorf("private key is required")
	}

	encryptedKey, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted symmetric key: %w", err)
	}

	symmetricKey, err := rsa.DecryptOAEP(
		sha256.New(),
		rand.Reader,
		privateKey,
		encryptedKey,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("decrypt symmetric key: %w", err)
	}

	if len(symmetricKey) != AES256KeySize {
		return nil, fmt.Errorf("invalid symmetric key size: got %d bytes, expected %d", len(symmetricKey), AES256KeySize)
	}

	return symmetricKey, nil
}

func (s SymmetricRepository) EncryptSymmetricKey(ctx context.Context, publicKey *rsa.PublicKey, symmetricKey []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	encryptedKey, err := rsa.EncryptOAEP(
		sha256.New(),
		rand.Reader,
		publicKey,
		symmetricKey,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("encrypt symmetric key: %w", err)
	}
	return encryptedKey, nil
}

func (s SymmetricRepository) GenerateSymmetricKey(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	symmetricKey := make([]byte, AES256KeySize)
	if _, err := rand.Read(symmetricKey); err != nil {
		return nil, fmt.Errorf("generate symmetric key: %w", err)
	}
	return symmetricKey, nil
}

func NewSymmetricRepository() *SymmetricRepository {
	return &SymmetricRepository{}
}
