package blob

import (
	"context"
	"errors"
	"io"
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
	List(ctx context.Context, prefix string) ([]string, error)
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
}

type TemporaryCleaner interface {
	DeleteTemporaries(ctx context.Context, prefix string) (int, error)
}
