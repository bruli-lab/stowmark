package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

func (h *Handler) Rewrap(ctx context.Context, oldPrivateKeyPat, newPublicKeyPath string) error {
	obsv, err := builtObservability(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	svc := repository.NewRewrap(h.folderRepository, h.symmetricKeyRepository, h.asymmetricKeyPaiRepository)
	tracerMdw := app.NewTracerCommandMiddleware(obsv.TracerProvider)
	handler := tracerMdw(app.NewRewrapKey(svc))
	_, err = handler.Handle(ctx, app.RewrapKeyCommand{
		RepositoryPath: h.repositoryPath,
		OldPrivateKey:  oldPrivateKeyPat,
		NewPublicKey:   newPublicKeyPath,
	})
	return err
}
