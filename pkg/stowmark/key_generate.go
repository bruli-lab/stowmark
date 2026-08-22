package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
)

func (h *Handler) KeyGenerate(ctx context.Context, folder string) (*encryption.AsymmetricKeyPair, error) {
	keys, err := encryption.NewAsymmetricKeyPair(folder)
	if err != nil {
		return nil, err
	}
	svc := encryption.NewCreateAsymmetricKeyPair(h.asymmetricKeyPaiRepository)
	return keys, svc.Create(ctx, keys)
}
