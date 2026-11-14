package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

const (
	defaultLockRefreshInterval = 5 * time.Minute
	lockReleaseTimeout         = 30 * time.Second
	unknownLockHostname        = "unknown"
)

var ErrLockLost = errors.New("repository lock lost")

type RepositoryLocker struct {
	repo            LockRepository
	owner           repository.LockOwner
	refreshInterval time.Duration
}

type LockerOption func(*RepositoryLocker)

func WithLockRefreshInterval(interval time.Duration) LockerOption {
	return func(l *RepositoryLocker) {
		if interval > 0 {
			l.refreshInterval = interval
		}
	}
}

func WithLockOwner(owner repository.LockOwner) LockerOption {
	return func(l *RepositoryLocker) {
		l.owner = owner
	}
}

func (l *RepositoryLocker) WithLock(ctx context.Context, kind repository.LockKind, operation func(context.Context) error) (resultErr error) {
	lock, err := l.acquire(ctx, kind)
	if err != nil {
		return err
	}

	lockCtx, cancel := context.WithCancelCause(ctx)
	heldLock := make(chan *repository.Lock, 1)
	go func() {
		heldLock <- l.keepAlive(lockCtx, lock, cancel)
	}()

	defer func() {
		cancel(nil)
		current := <-heldLock
		if cause := context.Cause(lockCtx); errors.Is(cause, ErrLockLost) {
			resultErr = errors.Join(cause, resultErr)
		}
		resultErr = errors.Join(resultErr, l.release(ctx, current))
	}()

	return operation(lockCtx)
}

func (l *RepositoryLocker) acquire(ctx context.Context, kind repository.LockKind) (*repository.Lock, error) {
	now := time.Now()
	lock, err := repository.NewLock(kind, l.owner, now)
	if err != nil {
		return nil, err
	}

	if err := l.repo.Save(ctx, lock); err != nil {
		return nil, fmt.Errorf("save repository lock: %w", err)
	}

	if err := l.checkConflicts(ctx, lock, now); err != nil {
		return nil, errors.Join(err, l.release(ctx, lock))
	}

	return lock, nil
}

func (l *RepositoryLocker) checkConflicts(ctx context.Context, lock *repository.Lock, now time.Time) error {
	locks, err := l.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("list repository locks: %w", err)
	}

	for _, other := range locks {
		if other.ID() == lock.ID() {
			continue
		}
		if other.IsStale(now) {
			if err := l.repo.Delete(ctx, other.ID()); err != nil {
				return fmt.Errorf("delete stale repository lock %s: %w", other.ID(), err)
			}
			continue
		}
		if lock.ConflictsWith(&other) {
			return repository.NewLockedError(&other)
		}
	}

	return nil
}

func (l *RepositoryLocker) keepAlive(ctx context.Context, lock *repository.Lock, lose context.CancelCauseFunc) *repository.Lock {
	ticker := time.NewTicker(l.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return lock
		case <-ticker.C:
			refreshed, err := l.refresh(ctx, lock)
			if err != nil {
				lose(fmt.Errorf("%w: %w", ErrLockLost, err))
				return lock
			}
			lock = refreshed
		}
	}
}

func (l *RepositoryLocker) refresh(ctx context.Context, lock *repository.Lock) (*repository.Lock, error) {
	locks, err := l.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list repository locks: %w", err)
	}
	if !slices.ContainsFunc(locks, func(other repository.Lock) bool { return other.ID() == lock.ID() }) {
		return nil, fmt.Errorf("lock %s was removed by another process", lock.ID())
	}

	refreshed := lock.Refresh(time.Now())
	if err := l.repo.Save(ctx, refreshed); err != nil {
		return nil, fmt.Errorf("save refreshed repository lock: %w", err)
	}
	if err := l.repo.Delete(ctx, lock.ID()); err != nil {
		return nil, errors.Join(fmt.Errorf("delete previous repository lock: %w", err), l.release(ctx, refreshed))
	}

	return refreshed, nil
}

func (l *RepositoryLocker) release(ctx context.Context, lock *repository.Lock) error {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lockReleaseTimeout)
	defer cancel()

	if err := l.repo.Delete(releaseCtx, lock.ID()); err != nil {
		return fmt.Errorf("release repository lock %s: %w", lock.ID(), err)
	}
	return nil
}

func NewRepositoryLocker(repo LockRepository, options ...LockerOption) *RepositoryLocker {
	locker := &RepositoryLocker{
		repo:            repo,
		owner:           repository.NewLockOwner(currentHostname(), os.Getpid()),
		refreshInterval: defaultLockRefreshInterval,
	}
	for _, option := range options {
		option(locker)
	}
	return locker
}

func currentHostname() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return unknownLockHostname
	}
	return hostname
}
