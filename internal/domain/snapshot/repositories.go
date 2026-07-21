package snapshot

import "context"

type SourceRepository interface {
	Explore(ctx context.Context, sourcePath string) (*Source, error)
	CalculateHash(ctx context.Context, filePath string) (string, error)
}

type ManifestRepository interface {
	Save(ctx context.Context, repositoryPath string, m *Manifest) error
}
