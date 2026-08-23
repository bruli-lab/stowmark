package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

const RewrapKeyCommandName = "rewrap-key"

type RewrapKeyCommand struct {
	RepositoryPath string
	OldPrivateKey  string
	NewPublicKey   string
}

func (r RewrapKeyCommand) Name() string {
	return RewrapKeyCommandName
}

type RewrapKey struct {
	svc *repository.Rewrap
}

func (r RewrapKey) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(RewrapKeyCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(RewrapKeyCommandName, cmd.Name())
	}
	return nil, r.svc.Do(ctx, co.RepositoryPath, co.OldPrivateKey, co.NewPublicKey)
}

func NewRewrapKey(svc *repository.Rewrap) *RewrapKey {
	return &RewrapKey{svc: svc}
}
