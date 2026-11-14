package stowmark

import (
	"context"
	"errors"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

type PruneResult struct {
	StoredObjects       int
	ReferencedObjects   int
	UnreferencedObjects int
	MissingObjects      int
	DryRun              bool
}

func (h *Handler) Prune(ctx context.Context, dryRun bool) (*PruneResult, error) {
	obsv, err := buildObservability(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	mdw, err := middlewares.BuildCommandMiddlewares(obsv, app.NewEventsTracing())
	if err != nil {
		return nil, err
	}
	handler := mdw(app.NewPruneRepository(h.folderRepository, h.manifestRepository, h.objectRepository, h.locker))
	events, err := handler.Handle(ctx, app.PruneRepositoryCommand{
		RepositoryPath: h.repositoryPath,
		DryRun:         dryRun,
	})
	if err != nil {
		return nil, err
	}
	if len(events) != 1 {
		return nil, errors.New("unexpected number of events")
	}
	result, ok := events[0].(*app.PruneRepositoryEvent)
	if !ok {
		return nil, fmt.Errorf("unexpected event type: %T", events[0])
	}
	return &PruneResult{
		StoredObjects:       result.StoredObjects,
		ReferencedObjects:   result.ReferencedObjects,
		UnreferencedObjects: result.UnreferencedObjects,
		MissingObjects:      result.MissingObjects,
		DryRun:              result.DryRun,
	}, nil
}
