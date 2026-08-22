package disk

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	PublicKey              = "PUBLIC KEY"
	PrivateKey             = "PRIVATE KEY"
	MinimumRSAKeySizeInBit = 2048
)

type AsymmetricKeyPairRepository struct{}

func (a AsymmetricKeyPairRepository) ReadRSAPrivateKey(ctx context.Context, filePath string) (*rsa.PrivateKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read private key %q: %w", filePath, err)
	}
	if len(data) == 0 {
		return nil, errors.New("private key is required")
	}

	block, rest := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid private key PEM")
	}

	if block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("unexpected PEM type %q", block.Type)
	}

	if len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("unexpected data after private key PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	privateKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}

	if err := privateKey.Validate(); err != nil {
		return nil, fmt.Errorf("validate private key: %w", err)
	}

	return privateKey, nil
}

func (a AsymmetricKeyPairRepository) ReadRSAPublicKey(ctx context.Context, filePath string) (*rsa.PublicKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read public key %q: %w", filePath, err)
	}

	block, rest := pem.Decode(data)
	if block == nil {
		return nil, errors.New("decode public key PEM: invalid PEM data")
	}

	if len(rest) != 0 {
		return nil, errors.New("decode public key PEM: unexpected trailing data")
	}

	var publicKey *rsa.PublicKey

	switch block.Type {
	case "PUBLIC KEY":
		parsedKey, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKIX public key: %w", err)
		}

		var ok bool
		publicKey, ok = parsedKey.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("public key is not an RSA key")
		}

	case "RSA PUBLIC KEY":
		parsedKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS#1 public key: %w", err)
		}

		publicKey = parsedKey

	default:
		return nil, fmt.Errorf("unsupported public key PEM type %q", block.Type)
	}

	if publicKey.N.BitLen() < MinimumRSAKeySizeInBit {
		return nil, fmt.Errorf(
			"RSA public key must have at least %d bits",
			MinimumRSAKeySizeInBit,
		)
	}

	return publicKey, nil
}

func (a AsymmetricKeyPairRepository) PublicKeyFingerPrint(ctx context.Context, publicKey *rsa.PublicKey) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}

	sum := sha256.Sum256(der)

	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:]), nil
}

func (a AsymmetricKeyPairRepository) CreatePrivateKey(ctx context.Context, filePath string, privateKey *rsa.PrivateKey) error {
	return a.createKey(ctx, filePath, PrivateKey, func() ([]byte, error) {
		return x509.MarshalPKCS8PrivateKey(privateKey)
	})
}

func (a AsymmetricKeyPairRepository) CreatePublicKey(ctx context.Context, filePath string, publicKey *rsa.PublicKey) error {
	return a.createKey(ctx, filePath, PublicKey, func() ([]byte, error) {
		return x509.MarshalPKIXPublicKey(publicKey)
	})
}

func (a AsymmetricKeyPairRepository) createKey(ctx context.Context, filePath, keyType string, marshal func() ([]byte, error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	directory := filepath.Dir(filePath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create key directory %q: %w", directory, err)
	}

	data, err := marshal()
	if err != nil {
		return fmt.Errorf("marshal %s: %w", keyType, err)
	}

	if err := a.writeFile(data, filePath, keyType); err != nil {
		return fmt.Errorf("write %s %q: %w", keyType, filePath, err)
	}

	return nil
}

func (a AsymmetricKeyPairRepository) writeFile(data []byte, filePath, fileType string) error {
	block := pem.EncodeToMemory(&pem.Block{
		Type:  fileType,
		Bytes: data,
	})

	return os.WriteFile(filePath, block, 0o644)
}

func NewAsymmetricKeyPairRepository() *AsymmetricKeyPairRepository {
	return &AsymmetricKeyPairRepository{}
}
