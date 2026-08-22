package encryption

import (
	"context"
	"crypto/rsa"
)

//go:generate go tool moq -out repositories_mock.go . AsymmetricKeyPairRepository SymmetricKeyRepository
type AsymmetricKeyPairRepository interface {
	CreatePrivateKey(ctx context.Context, filePath string, privateKey *rsa.PrivateKey) error
	CreatePublicKey(ctx context.Context, filePath string, publicKey *rsa.PublicKey) error
	ReadRSAPublicKey(ctx context.Context, filePath string) (*rsa.PublicKey, error)
	ReadRSAPrivateKey(ctx context.Context, filePath string) (*rsa.PrivateKey, error)
	PublicKeyFingerPrint(ctx context.Context, publicKey *rsa.PublicKey) (string, error)
}

type SymmetricKeyRepository interface {
	GenerateSymmetricKey(ctx context.Context) ([]byte, error)
	EncryptSymmetricKey(ctx context.Context, publicKey *rsa.PublicKey, symmetricKey []byte) ([]byte, error)
	DecodeAndDecryptSymmetricKey(ctx context.Context, privateKey *rsa.PrivateKey, encodedKey string) ([]byte, error)
}
