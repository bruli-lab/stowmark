package snapshot

import "context"

type SourceRepository interface {
	Explore(ctx context.Context, sourcePath string) (*Source, error)
	CalculateHash(ctx context.Context, filePath string) (string, error)
}
