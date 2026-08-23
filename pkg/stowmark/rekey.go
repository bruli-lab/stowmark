package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

func (h *Handler) ReKey(ctx context.Context, privateKeyPath, publicKeyPath string) error {
	obsv, err := builtObservability(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	svc := snapshot.NewReKey(h.folderRepository, h.symmetricKeyRepository, h.asymmetricKeyPaiRepository, h.objectRepository)
	mdw, err := middlewares.BuildCommandMiddlewares(obsv)
	if err != nil {
		return err
	}
	handler := mdw(app.NewRekeyKey(svc))
	_, err = handler.Handle(ctx, app.RekeyKeyCommand{
		RepositoryPath: h.repositoryPath,
		PrivateKeyPath: privateKeyPath,
		PublicKeyPath:  publicKeyPath,
	})
	if err != nil {
		return err
	}
	return nil
}
