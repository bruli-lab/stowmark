package stowmark

import (
	"context"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

type RetentionResult struct {
	Extended      int
	RetainedUntil time.Time
}

func (h *Handler) ExtendRetention(ctx context.Context) (*RetentionResult, error) {
	events, err := h.repository.ExtendRetention().Handle(ctx, app.ExtendRetentionCommand{
		RepositoryPath: h.repository.Path(),
		PrivateKeyPath: h.privateKey,
		Progress:       h.progress,
	})
	if err != nil {
		return nil, err
	}
	result := &RetentionResult{}
	for _, extended := range app.EventsOf[*repository.RetentionExtended](events) {
		result.Extended += extended.Extended()
		result.RetainedUntil = extended.Until()
	}
	return result, nil
}
