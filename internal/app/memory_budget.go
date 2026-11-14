package app

import (
	"context"

	"golang.org/x/sync/semaphore"
)

const defaultMemoryBudget int64 = 512 << 20

type memoryBudget struct {
	limit     int64
	semaphore *semaphore.Weighted
}

func newMemoryBudget(limit int64) *memoryBudget {
	if limit <= 0 {
		limit = defaultMemoryBudget
	}
	return &memoryBudget{limit: limit, semaphore: semaphore.NewWeighted(limit)}
}

func (m *memoryBudget) reserve(ctx context.Context, key objectKey, size int64) (release func(), err error) {
	if m == nil || key.symmetricKey == nil {
		return func() {}, nil
	}
	weight := min(max(size, 1), m.limit)
	if err := m.semaphore.Acquire(ctx, weight); err != nil {
		return nil, err
	}
	return func() { m.semaphore.Release(weight) }, nil
}
