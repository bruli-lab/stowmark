package app

import (
	"context"
	"errors"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

type objectVerifier struct {
	objectRepo ObjectRepository
	budget     *memoryBudget
}

func newObjectVerifier(objectRepo ObjectRepository) objectVerifier {
	return objectVerifier{objectRepo: objectRepo, budget: newMemoryBudget(defaultMemoryBudget)}
}

func (o objectVerifier) verify(ctx context.Context, expectedHash string, size int64, codec repository.Codec) (string, error) {
	if expectedHash == "" {
		return "object hash is empty", nil
	}
	release, err := o.budget.reserve(ctx, codec, size)
	if err != nil {
		return "", err
	}
	defer release()

	err = o.objectRepo.VerifyObject(ctx, expectedHash, codec)
	if err == nil {
		return "", nil
	}
	if errors.As(err, &snapshot.NotFoundError{}) || errors.Is(err, snapshot.ErrCorruptObject) {
		return err.Error(), nil
	}
	return "", err
}
