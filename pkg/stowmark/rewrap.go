package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

func (h *Handler) Rewrap(ctx context.Context, oldPrivateKeyPath, newPublicKeyPath string) error {
	obsv, err := buildObservability(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	evtr := app.NewEventsTracing()
	mdw, err := middlewares.BuildCommandMiddlewares(obsv, evtr)
	if err != nil {
		return err
	}
	handler := mdw(app.NewRewrapKey(h.folderRepository, h.asymmetricKeyPairRepository, h.symmetricKeyRepository, h.locker))
	_, err = handler.Handle(ctx, app.RewrapKeyCommand{
		RepositoryPath: h.repositoryPath,
		OldPrivateKey:  oldPrivateKeyPath,
		NewPublicKey:   newPublicKeyPath,
	})
	return err
}
