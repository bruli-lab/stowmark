package stowmark

import (
	"context"
	"fmt"
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

	obsv, err := builtObservability(ctx)
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
	handler := mdw(app.NewInit(h.folderRepository, h.asymmetricKeyPaiRepository, h.symmetricKeyRepository))
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
	if len(events) != 1 {
		return fmt.Errorf("unexpected number of init events: %d", len(events))
	}
	result, ok := events[0].(*app.InitEvent)
	if !ok {
		return fmt.Errorf("unexpected event type: %T", events[0])
	}
	for _, warning := range result.Warnings {
		slog.Warn(warning)
	}
	return nil
}
