package stowmark

import (
	"context"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

type UnlockResult struct {
	RemovedLocks int
	ActiveLocks  int
}

func (h *Handler) Unlock(ctx context.Context, all bool) (*UnlockResult, error) {
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
	events, err := mdw(app.NewUnlock(h.lockRepository)).Handle(ctx, app.UnlockCommand{
		RepositoryPath: h.repositoryPath,
		All:            all,
	})
	if err != nil {
		return nil, err
	}
	queryMdw, err := middlewares.BuildQueryMiddlewares(obsv)
	if err != nil {
		return nil, err
	}
	result, err := queryMdw(app.NewLockList(h.lockRepository)).Handle(ctx, app.LockListQuery{})
	if err != nil {
		return nil, err
	}
	active, ok := result.([]repository.Lock)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
	return &UnlockResult{
		RemovedLocks: len(app.EventsOf[*repository.LockRemoved](events)),
		ActiveLocks:  len(active),
	}, nil
}
