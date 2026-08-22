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
	Save(ctx context.Context, filePath, hash string, comp *repository.Compression, symmetricKey []byte, generation uint64) error
	AlreadyExists(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (bool, error)
	ReadObject(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (io.ReadCloser, error)
	RestoreObject(ctx context.Context, comp *repository.Compression, obj *File, symmetricKey []byte, generation uint64) error
	SaveChunk(ctx context.Context, filePath string, hash string, offset int64, size int64, comp *repository.Compression, key []byte, generation uint64) error
	ListEncryptedObjects(ctx context.Context, generation uint64) ([]string, error)
	ReadEncryptedObject(ctx context.Context, hash string, generation uint64, key []byte) (io.ReadCloser, error)
	SaveRekeyedObject(ctx context.Context, hash string, source io.Reader, generation uint64, key []byte) error
	DeleteEncryptedGeneration(ctx context.Context, generation uint64) error
	AbortRekey(ctx context.Context, generation uint64) error
}
