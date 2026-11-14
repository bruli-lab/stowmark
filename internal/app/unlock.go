package app

import (
	"context"
	"fmt"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/google/uuid"
)

const UnlockCommandName = "unlock"

type UnlockCommand struct {
	RepositoryPath string
	All            bool
}

func (u UnlockCommand) Name() string {
	return UnlockCommandName
}

type Unlock struct {
	lockRepo LockRepository
}

func (u *Unlock) Handle(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
	co, ok := cmd.(UnlockCommand)
	if !ok {
		return nil, cqs.NewInvalidCommandError(UnlockCommandName, cmd.Name())
	}
	locks, err := u.lockRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list repository locks: %w", err)
	}
	now := time.Now()
	removed := 0
	for i := range locks {
		if !co.All && !locks[i].IsStale(now) {
			continue
		}
		if err := u.lockRepo.Delete(ctx, locks[i].ID()); err != nil {
			return nil, fmt.Errorf("delete repository lock %s: %w", locks[i].ID(), err)
		}
		removed++
	}
	return []event.Event{
		NewUnlockEvent(co.RepositoryPath, removed, len(locks)-removed),
	}, nil
}

func NewUnlock(lockRepo LockRepository) *Unlock {
	return &Unlock{lockRepo: lockRepo}
}

type UnlockEvent struct {
	event.BasicEvent
	RemovedLocks int
	ActiveLocks  int
}

func NewUnlockEvent(repositoryPath string, removed, active int) *UnlockEvent {
	return &UnlockEvent{
		BasicEvent:   event.NewBasicEvent(UnlockCommandName, uuid.New(), repositoryPath),
		RemovedLocks: removed,
		ActiveLocks:  active,
	}
}
