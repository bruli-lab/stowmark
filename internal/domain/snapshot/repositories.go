package snapshot

import "context"

type SourceRepository interface {
	Explore(ctx context.Context, sourcePath string) (*Source, error)
	CalculateHash(ctx context.Context, filePath string) (string, error)
}

type ManifestRepository interface {
	Save(ctx context.Context, m *Manifest) error
}

type ObjectRepository interface {
	Save(ctx context.Context, obj *File) error
}
