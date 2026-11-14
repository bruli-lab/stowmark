package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

type CreateResult struct {
	ID        string
	FileCount int
	TotalSize int64
}

func (h *Handler) CreateSnapshot(ctx context.Context, source string, privateKey *string) (*CreateResult, error) {
	obsv, err := buildObservability(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	evtr := app.NewEventsTracing()
	evtr.Add(snapshot.SnapshotCreatedEventName, app.NewCreateSnapshotTracer(obsv.Logger))
	mdw, err := middlewares.BuildCommandMiddlewares(obsv, evtr)
	if err != nil {
		return nil, err
	}
	handler := mdw(app.NewCreateSnapshot(
		h.sourceRepository,
		h.manifestRepository,
		h.objectRepository,
		h.folderRepository,
		h.asymmetricKeyPairRepository,
		h.symmetricKeyRepository,
		h.locker,
	))

	events, err := handler.Handle(ctx, app.CreateSnapshotCommand{
		RepositoryPath: h.repositoryPath,
		SourcePath:     source,
		PrivateKey:     privateKey,
	})
	if err != nil {
		return nil, err
	}
	result, err := app.SingleEvent[*snapshot.SnapshotCreated](events)
	if err != nil {
		return nil, err
	}
	return &CreateResult{
		ID:        result.SnapshotID(),
		FileCount: result.FileCount(),
		TotalSize: result.TotalSize(),
	}, nil
}
