package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

type PrunePlan struct {
	StoredObjects       int
	ReferencedObjects   int
	UnreferencedObjects int
	MissingObjects      int
	OldKeyGenerations   []uint64
}

type PruneResult struct {
	RemovedObjects        int
	MissingObjects        int
	RemovedKeyGenerations []uint64
	RemovedTemporaryFiles int
}

func (h *Handler) PlanPrune(ctx context.Context) (*PrunePlan, error) {
	plan, err := app.QueryResult[*snapshot.PrunePlan](h.repository.PrunePlan().Handle(ctx, app.PrunePlanQuery{RepositoryPath: h.repository.Path()}))
	if err != nil {
		return nil, err
	}
	return &PrunePlan{
		StoredObjects:       plan.StoredObjects(),
		ReferencedObjects:   plan.ReferencedObjects(),
		UnreferencedObjects: len(plan.Unreferenced()),
		MissingObjects:      len(plan.Missing()),
		OldKeyGenerations:   plan.OrphanGenerations(),
	}, nil
}

func (h *Handler) Prune(ctx context.Context) (*PruneResult, error) {
	events, err := h.repository.PruneRepository().Handle(ctx, app.PruneRepositoryCommand{RepositoryPath: h.repository.Path()})
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
	for _, generations := range app.EventsOf[*snapshot.GenerationsRemoved](events) {
		result.RemovedKeyGenerations = append(result.RemovedKeyGenerations, generations.Generations()...)
	}
	for _, temporaries := range app.EventsOf[*snapshot.TemporariesRemoved](events) {
		result.RemovedTemporaryFiles += temporaries.Removed()
	}
	return result, nil
}
