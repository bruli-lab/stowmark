package repository

import (
	"time"

	"github.com/bruli-lab/go-core/event"
	"github.com/google/uuid"
)

const (
	RepositoryInitializedEventName = "repository.initialized"
	LockRemovedEventName           = "repository.lock_removed"
	RetentionExtendedEventName     = "repository.retention_extended"
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

type RetentionExtended struct {
	event.BasicEvent
	checked  int
	extended int
	until    time.Time
}

func (r *RetentionExtended) RepositoryID() string {
	return r.AggregateRootID()
}

func (r *RetentionExtended) Checked() int {
	return r.checked
}

func (r *RetentionExtended) Extended() int {
	return r.extended
}

func (r *RetentionExtended) Until() time.Time {
	return r.until
}

func (c *Config) RetentionExtended(checked, extended int, until time.Time) *RetentionExtended {
	return &RetentionExtended{
		BasicEvent: event.NewBasicEvent(RetentionExtendedEventName, uuid.New(), c.ID().String()),
		checked:    checked,
		extended:   extended,
		until:      until.UTC(),
	}
}
