package app

import (
	"context"
	"fmt"

	"github.com/bruli-lab/go-core/cqs"
)

const LockListQueryName = "lock-list"

type LockListQuery struct{}

func (l LockListQuery) Name() string {
	return LockListQueryName
}

type LockList struct {
	lockRepo LockRepository
}

func (l *LockList) Handle(ctx context.Context, query cqs.Query) (any, error) {
	if _, ok := query.(LockListQuery); !ok {
		return nil, cqs.NewInvalidQueryError(LockListQueryName, query.Name())
	}
	locks, err := l.lockRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list repository locks: %w", err)
	}
	return locks, nil
}

func NewLockList(lockRepo LockRepository) *LockList {
	return &LockList{lockRepo: lockRepo}
}
