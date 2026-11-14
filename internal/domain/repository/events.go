package repository

import (
	"github.com/bruli-lab/go-core/event"
	"github.com/google/uuid"
)

const (
	RepositoryInitializedEventName = "repository.initialized"
	LockRemovedEventName           = "repository.lock_removed"
)

type RepositoryInitialized struct {
	event.BasicEvent
	warnings []string
}

func (r *RepositoryInitialized) RepositoryID() string {
	return r.AggregateRootID()
}

func (r *RepositoryInitialized) Warnings() []string {
	return r.warnings
}

func (c *Config) Initialized(warnings []string) *RepositoryInitialized {
	return &RepositoryInitialized{
		BasicEvent: event.NewBasicEvent(RepositoryInitializedEventName, uuid.New(), c.ID().String()),
		warnings:   warnings,
	}
}

type LockRemoved struct {
	event.BasicEvent
	lock Lock
}

func (l *LockRemoved) Lock() *Lock {
	return &l.lock
}

func (l *Lock) Removed() *LockRemoved {
	return &LockRemoved{
		BasicEvent: event.NewBasicEvent(LockRemovedEventName, uuid.New(), l.ID().String()),
		lock:       *l,
	}
}
