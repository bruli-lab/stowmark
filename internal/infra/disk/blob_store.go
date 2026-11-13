package disk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/bruli-lab/stowmark/internal/infra/blob"
)

const (
	blobFileMode      os.FileMode = 0o644
	blobDirectoryMode os.FileMode = 0o755
)

type BlobStore struct {
	root string
}

func (s BlobStore) Put(ctx context.Context, key string, write func(io.Writer) error) error {
	blobPath, err := s.path(ctx, key)
	if err != nil {
		return err
	}

	exists, err := s.Exists(ctx, key)
	if err != nil || exists {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(blobPath), blobDirectoryMode); err != nil {
		return fmt.Errorf("create directory for %q: %w", blobPath, err)
	}

	return writeFileAtomically(blobPath, blobFileMode, func(destination io.Writer) error {
		if err := write(destination); err != nil {
			return err
		}
		return ctx.Err()
	})
}

func (s BlobStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	blobPath, err := s.path(ctx, key)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(blobPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %q", blob.ErrNotFound, key)
		}
		return nil, fmt.Errorf("open %q: %w", blobPath, err)
	}

	return file, nil
}

func (s BlobStore) Exists(ctx context.Context, key string) (bool, error) {
	blobPath, err := s.path(ctx, key)
	if err != nil {
		return false, err
	}

	_, err = os.Stat(blobPath)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("stat %q: %w", blobPath, err)
	}
}

func (s BlobStore) List(ctx context.Context, prefix string) ([]string, error) {
	prefixPath, err := s.path(ctx, prefix)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0)
	err = filepath.WalkDir(prefixPath, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || isTemporaryFile(entry.Name()) {
			return nil
		}

		relativePath, err := filepath.Rel(s.root, filePath)
		if err != nil {
			return fmt.Errorf("relative path of %q: %w", filePath, err)
		}
		keys = append(keys, filepath.ToSlash(relativePath))

		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("list %q: %w", prefixPath, err)
	}
	sort.Strings(keys)

	return keys, nil
}

func (s BlobStore) DeletePrefix(ctx context.Context, prefix string) error {
	prefixPath, err := s.path(ctx, prefix)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(prefixPath); err != nil {
		return fmt.Errorf("delete %q: %w", prefixPath, err)
	}

	return nil
}

func (s BlobStore) path(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := blob.ValidateKey(key); err != nil {
		return "", err
	}

	return filepath.Join(s.root, filepath.FromSlash(key)), nil
}

func NewBlobStore(repositoryPath string) (*BlobStore, error) {
	root, err := absolutePath(repositoryPath)
	if err != nil {
		return nil, err
	}

	return &BlobStore{root: root}, nil
}
