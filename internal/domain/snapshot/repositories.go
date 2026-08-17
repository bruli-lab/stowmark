package snapshot

import (
	"context"
	"io"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

//go:generate go tool moq -out repositories_mock.go . SourceRepository ManifestRepository ObjectRepository
type SourceRepository interface {
	Explore(ctx context.Context, sourcePath string) (*Source, error)
	CalculateHash(ctx context.Context, filePath string, comp *repository.Compression) (string, error)
	CalculateChunks(ctx context.Context, filePath string, size int64, comp *repository.Compression) ([]Chunk, error)
}

type ManifestRepository interface {
	Save(ctx context.Context, m *Manifest) error
	List(ctx context.Context) ([]Manifest, error)
	Get(ctx context.Context, snapshotID string) (*Manifest, error)
}

type ObjectRepository interface {
	Save(ctx context.Context, filePath, hash string, comp *repository.Compression) error
	AlreadyExists(ctx context.Context, hash string) (bool, error)
	ReadObject(ctx context.Context, hash string) (io.ReadCloser, error)
	RestoreObject(ctx context.Context, comp *repository.Compression, obj *File) error
	SaveChunk(ctx context.Context, filePath string, hash string, offset int64, size int64, comp *repository.Compression) error
}
