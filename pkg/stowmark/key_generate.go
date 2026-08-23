package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

func (h *Handler) KeyGenerate(ctx context.Context, folder string) (*encryption.AsymmetricKeyPair, error) {
	obsv, err := builtObservability(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	keys, err := encryption.NewAsymmetricKeyPair(folder)
	if err != nil {
		return nil, err
	}
	svc := encryption.NewCreateAsymmetricKeyPair(h.asymmetricKeyPaiRepository)
	mdw, err := middlewares.BuildCommandMiddlewares(obsv)
	if err != nil {
		return nil, err
	}
	handler := mdw(app.NewGenerateKey(svc))
	_, err = handler.Handle(ctx, app.GenerateKeyCommand{Keys: keys})
	if err != nil {
		return nil, err
	}
	return keys, nil
}
