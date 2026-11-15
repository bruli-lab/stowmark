package app

import (
	"context"
	"fmt"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
)

const GenerateKeyCommandName = "generate-key"

type GenerateKeyCommand struct {
	Keys *encryption.AsymmetricKeyPair
}

func (g GenerateKeyCommand) Name() string {
	return GenerateKeyCommandName
}

type GenerateKey struct {
	generator      KeyGenerator
	asymmetricRepo AsymmetricKeyPairRepository
}

func (g *GenerateKey) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(GenerateKeyCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(GenerateKeyCommandName, cmd.Name())
	}
	privateKey, err := g.generator.GenerateRSAKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	if err := g.asymmetricRepo.CreatePrivateKey(ctx, co.Keys.PrivateKey(), privateKey); err != nil {
		return nil, err
	}
	return nil, g.asymmetricRepo.CreatePublicKey(ctx, co.Keys.PublicKey(), &privateKey.PublicKey)
}

func NewGenerateKey(generator KeyGenerator, asymmetricRepo AsymmetricKeyPairRepository) *GenerateKey {
	return &GenerateKey{generator: generator, asymmetricRepo: asymmetricRepo}
}
