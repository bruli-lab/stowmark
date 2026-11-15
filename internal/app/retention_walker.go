package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"golang.org/x/sync/errgroup"
)

const defaultRetentionWorkers = 16

type retentionWalker struct {
	retentionRepo RetentionRepository
	workers       int
}

func (w retentionWalker) walk(
	ctx context.Context,
	items []repository.RetainedItem,
	progress Progress,
	visit func(ctx context.Context, item repository.RetainedItem, until time.Time) error,
) error {
	if len(items) == 0 {
		return nil
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(min(w.workers, len(items)))

	for _, item := range items {
		if groupCtx.Err() != nil {
			break
		}

		group.Go(func() error {
			defer progress.Advance(1, 0)
			until, err := w.retentionRepo.RetainedUntil(groupCtx, item)
			if _, missing := errors.AsType[*repository.NotFoundError](err); missing {
				return nil
			}
			if err != nil {
				return fmt.Errorf("read the retention of %s %q: %w", item.Kind(), item.ID(), err)
			}
			return visit(groupCtx, item, until)
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}

	return ctx.Err()
}

func newRetentionWalker(retentionRepo RetentionRepository) retentionWalker {
	return retentionWalker{retentionRepo: retentionRepo, workers: defaultRetentionWorkers}
}
