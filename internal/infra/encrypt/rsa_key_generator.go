package encrypt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
)

const RSAKeySize = 3072

type RSAKeyGenerator struct{}

func (RSAKeyGenerator) GenerateRSAKey(ctx context.Context) (*rsa.PrivateKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, RSAKeySize)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}
	if err := privateKey.Validate(); err != nil {
		return nil, fmt.Errorf("validate RSA key: %w", err)
	}
	return privateKey, nil
}

func NewRSAKeyGenerator() RSAKeyGenerator {
	return RSAKeyGenerator{}
}
