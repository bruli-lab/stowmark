package repository

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const LockStaleAfter = 30 * time.Minute

var (
	ErrRepositoryLocked   = errors.New("repository is locked")
	ErrInvalidLockKind    = errors.New("invalid lock kind")
	ErrInvalidLockID      = errors.New("invalid lock id")
	ErrMissingLockOwner   = errors.New("missing lock owner")
	ErrMissingLockCreated = errors.New("missing lock creation date")
)

type LockKind string

const (
	SharedLock    LockKind = "shared"
	ExclusiveLock LockKind = "exclusive"
)

func (k LockKind) String() string {
	return string(k)
}

func ParseLockKind(value string) (LockKind, error) {
	kind := LockKind(value)
	switch kind {
	case SharedLock, ExclusiveLock:
		return kind, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidLockKind, value)
	}
}

type LockOwner struct {
	hostname string
	pid      int
}

func (o LockOwner) Hostname() string {
	return o.hostname
}

func (o LockOwner) PID() int {
	return o.pid
}

func NewLockOwner(hostname string, pid int) LockOwner {
	return LockOwner{hostname: hostname, pid: pid}
}

type Lock struct {
	id        uuid.UUID
	kind      LockKind
	owner     LockOwner
	createdAt time.Time
}

func (l *Lock) ID() uuid.UUID {
	return l.id
}

func (l *Lock) Kind() LockKind {
	return l.kind
}

func (l *Lock) Owner() LockOwner {
	return l.owner
}

func (l *Lock) CreatedAt() time.Time {
	return l.createdAt
}

func (l *Lock) IsStale(now time.Time) bool {
	return now.Sub(l.createdAt) > LockStaleAfter
}

func (l *Lock) ConflictsWith(other *Lock) bool {
	if l.id == other.id {
		return false
	}
	return l.kind == ExclusiveLock || other.kind == ExclusiveLock
}

func (l *Lock) Refresh(now time.Time) *Lock {
	return &Lock{id: uuid.New(), kind: l.kind, owner: l.owner, createdAt: now.UTC()}
}

func NewLock(kind LockKind, owner LockOwner, now time.Time) (*Lock, error) {
	return HydrateLock(uuid.New(), kind, owner, now)
}

func HydrateLock(id uuid.UUID, kind LockKind, owner LockOwner, createdAt time.Time) (*Lock, error) {
	if _, err := ParseLockKind(kind.String()); err != nil {
		return nil, err
	}
	switch {
	case id == uuid.Nil:
		return nil, ErrInvalidLockID
	case owner.hostname == "":
		return nil, ErrMissingLockOwner
	case createdAt.IsZero():
		return nil, ErrMissingLockCreated
	}
	return &Lock{id: id, kind: kind, owner: owner, createdAt: createdAt.UTC()}, nil
}

type LockedError struct {
	holder *Lock
}

func (e LockedError) Error() string {
	return fmt.Sprintf(
		"%s: %s lock held by %s (pid %d), last refreshed at %s; it becomes stale %s after its last refresh",
		ErrRepositoryLocked,
		e.holder.kind,
		e.holder.owner.hostname,
		e.holder.owner.pid,
		e.holder.createdAt.Format(time.RFC3339),
		LockStaleAfter,
	)
}

func (e LockedError) Unwrap() error {
	return ErrRepositoryLocked
}

func (e LockedError) Holder() *Lock {
	return e.holder
}

func NewLockedError(holder *Lock) LockedError {
	return LockedError{holder: holder}
}
