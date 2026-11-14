package stowmark

import (
	"context"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

type PrunePlan struct {
	StoredObjects       int
	ReferencedObjects   int
	UnreferencedObjects int
	MissingObjects      int
}

type PruneResult struct {
	RemovedObjects int
	MissingObjects int
}

func (h *Handler) PlanPrune(ctx context.Context) (*PrunePlan, error) {
	obsv, err := buildObservability(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = obsv.Shutdown(ctx)
	}()
	mdw, err := middlewares.BuildQueryMiddlewares(obsv)
	if err != nil {
		return nil, err
	}
	handler := mdw(app.NewPrunePlan(h.folderRepository, h.manifestRepository, h.objectRepository, h.locker))
	result, err := handler.Handle(ctx, app.PrunePlanQuery{RepositoryPath: h.repositoryPath})
	if err != nil {
		return nil, err
	}
	plan, ok := result.(*snapshot.PrunePlan)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
	return &PrunePlan{
		StoredObjects:       plan.StoredObjects(),
		ReferencedObjects:   plan.ReferencedObjects(),
		UnreferencedObjects: len(plan.Unreferenced()),
		MissingObjects:      len(plan.Missing()),
	}, nil
}

func (h *Handler) Prune(ctx context.Context) (*PruneResult, error) {
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
	events, err := handler.Handle(ctx, app.PruneRepositoryCommand{RepositoryPath: h.repositoryPath})
	if err != nil {
		return nil, err
	}
	result := &PruneResult{}
	for _, pruned := range app.EventsOf[*snapshot.ObjectsPruned](events) {
		result.RemovedObjects += pruned.Removed()
	}
	for _, missing := range app.EventsOf[*snapshot.MissingObjectsDetected](events) {
		result.MissingObjects += len(missing.Hashes())
	}
	return result, nil
}
