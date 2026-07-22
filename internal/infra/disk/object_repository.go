package disk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bruli-lab/stowmark.git/internal/domain/repository"
	"github.com/bruli-lab/stowmark.git/internal/domain/snapshot"
)

type ObjectRepository struct {
	repositoryPath string
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, obj *snapshot.File) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}
	hash := obj.Hash()
	destinationPath := filepath.Join(o.repositoryPath, repository.ObjectsFolder, hash[:2], hash[2:])
	_, err := os.Stat(destinationPath)
	switch {
	case err == nil:
		return true, nil
	case !errors.Is(err, os.ErrNotExist):
		return false, fmt.Errorf("check destination file %q: %w", destinationPath, err)
	}
	return false, nil
}

func (o ObjectRepository) Save(ctx context.Context, obj *snapshot.File) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	hash := obj.Hash()
	destinationPath := filepath.Join(o.repositoryPath, repository.ObjectsFolder, hash[:2], hash[2:])
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return fmt.Errorf(
			"create object directory %q: %w",
			filepath.Dir(destinationPath),
			err,
		)
	}
	source, err := os.Open(obj.Path())
	if err != nil {
		return fmt.Errorf("open source file %q: %w", obj.Path(), err)
	}
	defer func() {
		_ = source.Close()
	}()
	destination, err := os.OpenFile(
		destinationPath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0o644,
	)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}

		return fmt.Errorf(
			"create object file %q: %w",
			destinationPath,
			err,
		)
	}
	copyCompleted := false
	defer func() {
		_ = destination.Close()

		if !copyCompleted {
			_ = os.Remove(destinationPath)
		}
	}()
	if _, err := io.Copy(destination, source); err != nil {
		return fmt.Errorf(
			"copy %q to %q: %w",
			obj.Path(),
			destinationPath,
			err,
		)
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf(
			"close destination file %q: %w",
			destinationPath,
			err,
		)
	}
	copyCompleted = true
	return nil
}

func NewObjectRepository(repositoryPath string) *ObjectRepository {
	return &ObjectRepository{repositoryPath: repositoryPath}
}
