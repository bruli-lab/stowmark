package object

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

func GetPath(ctx context.Context, repositoryPath string, comp *repository.Compression, obj *snapshot.File) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if comp == nil {
		return "", errors.New("compression configuration is required")
	}

	if obj == nil {
		return "", errors.New("snapshot object is required")
	}

	hash := obj.Hash()
	if len(hash) < 3 {
		return "", fmt.Errorf("invalid object hash %q", hash)
	}

	return path.Join(
		repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	), nil
}
