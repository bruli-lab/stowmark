package snapshot

import "context"

//go:generate go tool moq -out repositories_mock.go . SourceRepository ManifestRepository ObjectRepository
type SourceRepository interface {
	Explore(ctx context.Context, sourcePath string) (*Source, error)
	CalculateHash(ctx context.Context, filePath string) (string, error)
}

type ManifestRepository interface {
	Save(ctx context.Context, m *Manifest) error
	List(ctx context.Context) ([]Manifest, error)
}

type ObjectRepository interface {
	Save(ctx context.Context, obj *File) error
	AlreadyExists(ctx context.Context, obj *File) (bool, error)
}
