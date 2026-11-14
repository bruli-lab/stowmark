package blob

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
)

var (
	ErrNotFound    = errors.New("blob not found")
	ErrInvalidKey  = errors.New("invalid blob key")
	ErrInvalidHash = errors.New("invalid object hash")
)

type Store interface {
	Put(ctx context.Context, key string, write func(io.Writer) error) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Exists(ctx context.Context, key string) (bool, error)
	List(ctx context.Context, prefix string) ([]Entry, error)
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
}

type Entry struct {
	Key  string
	Size int64
}

func SortEntries(entries []Entry) {
	slices.SortFunc(entries, func(a, b Entry) int { return strings.Compare(a.Key, b.Key) })
}

func Keys(entries []Entry) []string {
	keys := make([]string, len(entries))
	for i, entry := range entries {
		keys[i] = entry.Key
	}
	return keys
}

type TemporaryCleaner interface {
	DeleteTemporaries(ctx context.Context, prefix string) (int, error)
}
