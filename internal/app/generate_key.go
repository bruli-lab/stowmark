package app

import (
	"context"

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
	svc *encryption.CreateAsymmetricKeyPair
}

func (g GenerateKey) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(GenerateKeyCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(GenerateKeyCommandName, cmd.Name())
	}
	if err := g.svc.Create(ctx, co.Keys); err != nil {
		return nil, err
	}
	return nil, nil
}

func NewGenerateKey(svc *encryption.CreateAsymmetricKeyPair) *GenerateKey {
	return &GenerateKey{svc: svc}
}