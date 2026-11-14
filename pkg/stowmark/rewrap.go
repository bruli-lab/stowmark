package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
)

func (h *Handler) Rewrap(ctx context.Context, oldPrivateKeyPath, newPublicKeyPath string) error {
	_, err := h.repository.RewrapKey().Handle(ctx, app.RewrapKeyCommand{
		RepositoryPath: h.repository.Path(),
		OldPrivateKey:  oldPrivateKeyPath,
		NewPublicKey:   newPublicKeyPath,
	})
	return err
}
