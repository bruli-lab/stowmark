package stowmark

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

type UnlockResult struct {
	RemovedLocks int
	ActiveLocks  int
}

func (h *Handler) Unlock(ctx context.Context, all bool) (*UnlockResult, error) {
	events, err := h.repository.Unlock().Handle(ctx, app.UnlockCommand{
		RepositoryPath: h.repository.Path(),
		All:            all,
	})
	if err != nil {
		return nil, err
	}
	active, err := app.QueryResult[[]repository.Lock](h.repository.LockList().Handle(ctx, app.LockListQuery{}))
	if err != nil {
		return nil, err
	}
	return &UnlockResult{
		RemovedLocks: len(app.EventsOf[*repository.LockRemoved](events)),
		ActiveLocks:  len(active),
	}, nil
}
