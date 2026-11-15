package blob

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

type RetentionRepository struct {
	store Store
}

func NewRetentionRepository(store Store) *RetentionRepository {
	return &RetentionRepository{store: store}
}

func (r RetentionRepository) VerifyObjectLock(ctx context.Context) error {
	retained, ok := r.store.(RetentionStore)
	if !ok {
		return repository.ErrObjectLockUnsupported
	}
	return retained.VerifyObjectLock(ctx)
}

func (r RetentionRepository) RetainedUntil(ctx context.Context, item repository.RetainedItem) (time.Time, error) {
	retained, key, err := r.target(item)
	if err != nil {
		return time.Time{}, err
	}
	until, err := retained.RetainedUntil(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return time.Time{}, repository.NewNotFoundError(fmt.Sprintf("%s %q", item.Kind(), item.ID()))
	}
	return until, err
}

func (r RetentionRepository) Retain(ctx context.Context, item repository.RetainedItem, retention repository.Retention) error {
	retained, key, err := r.target(item)
	if err != nil {
		return err
	}
	err = retained.Retain(ctx, key, retention)
	if errors.Is(err, ErrNotFound) {
		return repository.NewNotFoundError(fmt.Sprintf("%s %q", item.Kind(), item.ID()))
	}
	return err
}

func (r RetentionRepository) target(item repository.RetainedItem) (RetentionStore, string, error) {
	retained, ok := r.store.(RetentionStore)
	if !ok {
		return nil, "", repository.ErrObjectLockUnsupported
	}
	switch item.Kind() {
	case repository.RetainedObject:
		key, err := objectKey(item.ID(), item.Encrypted(), item.Generation())
		return retained, key, err
	case repository.RetainedManifest:
		key, err := manifestKey(item.ID())
		return retained, key, err
	case repository.RetainedConfig:
		return retained, model.ConfigFile, nil
	default:
		return nil, "", fmt.Errorf("%w: unknown retained item %q", ErrInvalidKey, item.Kind())
	}
}
