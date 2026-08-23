//nolint:dupl // CQS command handlers intentionally share the same adapter structure.
package app

import (
	"context"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

const RekeyKeyCommandName = "rekey-key"

type RekeyKeyCommand struct {
	RepositoryPath string
	PrivateKeyPath string
	PublicKeyPath  string
}

func (r RekeyKeyCommand) Name() string {
	return RekeyKeyCommandName
}

type RekeyKey struct {
	svc *snapshot.ReKey
}

func (r RekeyKey) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(RekeyKeyCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(RekeyKeyCommandName, cmd.Name())
	}
	return nil, r.svc.Do(ctx, co.RepositoryPath, co.PrivateKeyPath, co.PublicKeyPath)
}

func NewRekeyKey(svc *snapshot.ReKey) *RekeyKey {
	return &RekeyKey{svc: svc}
}
