package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
)

func (h *Handler) KeyGenerate(ctx context.Context, folder string) (*encryption.AsymmetricKeyPair, error) {
	keys, err := encryption.NewAsymmetricKeyPair(folder)
	if err != nil {
		return nil, err
	}
	if _, err := h.runtime.GenerateKey().Handle(ctx, app.GenerateKeyCommand{Keys: keys}); err != nil {
		return nil, err
	}
	return keys, nil
}
