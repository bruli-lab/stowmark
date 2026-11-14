package stowmark

import (
	"context"
	"errors"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
)

type UnlockResult struct {
	RemovedLocks int
	ActiveLocks  int
}

func (h *Handler) Unlock(ctx context.Context, all bool) (*UnlockResult, error) {
	obsv, err := builtObservability(ctx)
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
	if len(events) != 1 {
		return nil, errors.New("unexpected number of events")
	}
	result, ok := events[0].(*app.UnlockEvent)
	if !ok {
		return nil, fmt.Errorf("unexpected event type: %T", events[0])
	}
	return &UnlockResult{RemovedLocks: result.RemovedLocks, ActiveLocks: result.ActiveLocks}, nil
}
