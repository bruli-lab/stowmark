package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
)

const (
	GenerateKeyCommandName = "generate-key"
	rsaKeySize             = 3072
)

type GenerateKeyCommand struct {
	Keys *encryption.AsymmetricKeyPair
}

func (g GenerateKeyCommand) Name() string {
	return GenerateKeyCommandName
}

type GenerateKey struct {
	asymmetricRepo AsymmetricKeyPairRepository
}

func (g *GenerateKey) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(GenerateKeyCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(GenerateKeyCommandName, cmd.Name())
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	if err := privateKey.Validate(); err != nil {
		return nil, fmt.Errorf("private key is invalid: %w", err)
	}
	if err := g.asymmetricRepo.CreatePrivateKey(ctx, co.Keys.PrivateKey(), privateKey); err != nil {
		return nil, err
	}
	return nil, g.asymmetricRepo.CreatePublicKey(ctx, co.Keys.PublicKey(), &privateKey.PublicKey)
}

func NewGenerateKey(asymmetricRepo AsymmetricKeyPairRepository) *GenerateKey {
	return &GenerateKey{asymmetricRepo: asymmetricRepo}
}
