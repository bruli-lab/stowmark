package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
)

func (h *Handler) ReKey(ctx context.Context, privateKeyPath, publicKeyPath string) error {
	_, err := h.repository.RekeyKey().Handle(ctx, app.RekeyKeyCommand{
		RepositoryPath: h.repository.Path(),
		PrivateKeyPath: privateKeyPath,
		PublicKeyPath:  publicKeyPath,
	})
	return err
}
