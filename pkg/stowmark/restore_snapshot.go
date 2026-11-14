package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

func (h *Handler) RestoreSnapshot(ctx context.Context, snapshotID string, destinationPath, privateKey *string) (*Result, error) {
	obsv, err := buildObservability(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	restoreSnapshotTracer := app.NewRestoreSnapshotTracer(obsv.Logger)
	evtr := app.NewEventsTracing()
	evtr.Add(snapshot.SnapshotRestoredEventName, restoreSnapshotTracer)
	mdw, err := middlewares.BuildCommandMiddlewares(obsv, evtr)
	if err != nil {
		return nil, err
	}
	handler := mdw(app.NewRestoreSnapshot(h.manifestRepository, h.objectRepository, h.folderRepository, h.asymmetricKeyPairRepository, h.symmetricKeyRepository, h.locker))
	events, err := handler.Handle(ctx, app.RestoreSnapshotCommand{
		SnapshotID:      snapshotID,
		RepositoryPath:  h.repositoryPath,
		PrivateKeyPath:  privateKey,
		DestinationPath: destinationPath,
	})
	if err != nil {
		return nil, err
	}
	result, err := app.SingleEvent[*snapshot.SnapshotRestored](events)
	if err != nil {
		return nil, err
	}
	failed := make([]FailedResult, len(result.Failed()))
	for i, f := range result.Failed() {
		failed[i] = FailedResult{
			Path:   f.Path(),
			Reason: f.Reason(),
		}
	}
	return &Result{
		SnapshotID: result.SnapshotID(),
		TotalFiles: result.TotalFiles(),
		Failed:     failed,
		IsSuccess:  result.IsSuccess(),
	}, nil
}
