package blob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/google/uuid"
)

const lockExtension = ".json"

type LockRepository struct {
	store Store
}

func (l LockRepository) Save(ctx context.Context, lock *repository.Lock) error {
	data, err := json.Marshal(model.NewLock(lock))
	if err != nil {
		return fmt.Errorf("marshal lock %s: %w", lock.ID(), err)
	}

	return l.store.Put(ctx, lockKey(lock.ID()), func(destination io.Writer) error {
		_, err := destination.Write(append(data, '\n'))
		return err
	})
}

func (l LockRepository) List(ctx context.Context) ([]repository.Lock, error) {
	keys, err := l.store.List(ctx, repository.LocksFolder)
	if err != nil {
		return nil, fmt.Errorf("list locks: %w", err)
	}

	locks := make([]repository.Lock, 0, len(keys))
	for _, key := range keys {
		if !isLockKey(key) {
			continue
		}
		lock, err := l.read(ctx, key)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		locks = append(locks, *lock)
	}

	return locks, nil
}

func (l LockRepository) Delete(ctx context.Context, lockID uuid.UUID) error {
	return l.store.Delete(ctx, lockKey(lockID))
}

func (l LockRepository) read(ctx context.Context, key string) (*repository.Lock, error) {
	reader, err := l.store.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()

	data, err := io.ReadAll(model.ContextReader{Ctx: ctx, Reader: reader})
	if err != nil {
		return nil, fmt.Errorf("read lock %q: %w", key, err)
	}

	return model.BuildLockDomain(data, key)
}

func lockKey(lockID uuid.UUID) string {
	return path.Join(repository.LocksFolder, lockID.String()+lockExtension)
}

func isLockKey(key string) bool {
	relative, ok := strings.CutPrefix(key, repository.LocksFolder+"/")
	return ok && !strings.Contains(relative, "/") && strings.HasSuffix(relative, lockExtension)
}

func NewLockRepository(store Store) *LockRepository {
	return &LockRepository{store: store}
}
