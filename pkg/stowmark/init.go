package stowmark

import (
	"context"
	"log/slog"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

type Compression struct {
	Type  string
	Level *int
}

func (h *Handler) Init(ctx context.Context, comp *Compression, formatVersion int, publicKey *string, force bool) error {
	compType, err := repository.ParseCompressionType(comp.Type)
	if err != nil {
		return err
	}
	compre, err := repository.NewCompression(*compType, comp.Level)
	if err != nil {
		return err
	}
	version, err := repository.ParseFormatVersionOrCurrent(formatVersion)
	if err != nil {
		return err
	}

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
	handler := mdw(app.NewInit(h.folderRepository, h.asymmetricKeyPairRepository, h.symmetricKeyRepository, h.locker))
	events, err := handler.Handle(ctx, app.InitCommand{
		RepositoryPath: h.repositoryPath,
		Compression:    compre,
		FormatVersion:  version,
		PublicKeyPath:  publicKey,
		Force:          force,
	})
	if err != nil {
		return err
	}
	result, err := app.SingleEvent[*repository.RepositoryInitialized](events)
	if err != nil {
		return err
	}
	for _, warning := range result.Warnings() {
		slog.Warn(warning)
	}
	return nil
}
