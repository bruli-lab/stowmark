package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/google/uuid"
)

type Lock struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Hostname  string    `json:"hostname"`
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
}

func NewLock(lock *repository.Lock) Lock {
	return Lock{
		ID:        lock.ID().String(),
		Kind:      lock.Kind().String(),
		Hostname:  lock.Owner().Hostname(),
		PID:       lock.Owner().PID(),
		CreatedAt: lock.CreatedAt(),
	}
}

func BuildLockDomain(data []byte, lockPath string) (*repository.Lock, error) {
	var lo Lock
	if err := json.Unmarshal(data, &lo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal lock %q: %w", lockPath, err)
	}
	id, err := uuid.Parse(lo.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid lock id in %q: %w", lockPath, err)
	}
	kind, err := repository.ParseLockKind(lo.Kind)
	if err != nil {
		return nil, fmt.Errorf("invalid lock %q: %w", lockPath, err)
	}
	lock, err := repository.HydrateLock(id, kind, repository.NewLockOwner(lo.Hostname, lo.PID), lo.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid lock %q: %w", lockPath, err)
	}
	return lock, nil
}
