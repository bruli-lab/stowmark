package blobtest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/bruli-lab/stowmark/internal/infra/blob"
)

type MemoryStore struct {
	mu    sync.RWMutex
	blobs map[string][]byte
}

func (m *MemoryStore) Put(ctx context.Context, key string, write func(io.Writer) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := blob.ValidateKey(key); err != nil {
		return err
	}
	if exists, _ := m.Exists(ctx, key); exists {
		return nil
	}

	var buffer bytes.Buffer
	if err := write(&buffer); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.blobs[key]; !ok {
		m.blobs[key] = buffer.Bytes()
	}

	return nil
}

func (m *MemoryStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := blob.ValidateKey(key); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.blobs[key]
	if !ok {
		return nil, fmt.Errorf("%w: %q", blob.ErrNotFound, key)
	}

	return io.NopCloser(bytes.NewReader(bytes.Clone(data))), nil
}

func (m *MemoryStore) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := blob.ValidateKey(key); err != nil {
		return false, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.blobs[key]

	return ok, nil
}

func (m *MemoryStore) List(ctx context.Context, prefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := blob.ValidateKey(prefix); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, 0)
	for key := range m.blobs {
		if strings.HasPrefix(key, prefix+"/") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	return keys, nil
}

func (m *MemoryStore) DeletePrefix(ctx context.Context, prefix string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := blob.ValidateKey(prefix); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.blobs {
		if strings.HasPrefix(key, prefix+"/") {
			delete(m.blobs, key)
		}
	}

	return nil
}

func (m *MemoryStore) Corrupt(key string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blobs[key] = data
}

func (m *MemoryStore) Keys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, 0, len(m.blobs))
	for key := range m.blobs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{blobs: make(map[string][]byte)}
}
