package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/pkg/sftp"
)

type BlobStore struct {
	client *sftp.Client
	root   string
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

	directory := path.Dir(blobPath)
	if err := s.client.MkdirAll(directory); err != nil {
		return fmt.Errorf("create SSH directory %q: %w", directory, err)
	}

	return writeFileAtomically(s.client, blobPath, func(destination io.Writer) error {
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

	file, err := s.client.Open(blobPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %q", blob.ErrNotFound, key)
		}
		return nil, fmt.Errorf("open SSH file %q: %w", blobPath, err)
	}

	return file, nil
}

func (s BlobStore) Exists(ctx context.Context, key string) (bool, error) {
	blobPath, err := s.path(ctx, key)
	if err != nil {
		return false, err
	}

	_, err = s.client.Stat(blobPath)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("stat SSH file %q: %w", blobPath, err)
	}
}

func (s BlobStore) List(ctx context.Context, prefix string) ([]string, error) {
	prefixPath, err := s.path(ctx, prefix)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0)
	walker := s.client.Walk(prefixPath)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			if errors.Is(err, fs.ErrNotExist) && walker.Path() == prefixPath {
				return []string{}, nil
			}
			return nil, fmt.Errorf("list SSH directory %q: %w", walker.Path(), err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if walker.Stat().IsDir() || isTemporaryFile(walker.Stat().Name()) {
			continue
		}
		keys = append(keys, path.Join(prefix, strings.TrimPrefix(walker.Path(), prefixPath)))
	}
	sort.Strings(keys)

	return keys, nil
}

func (s BlobStore) Delete(ctx context.Context, key string) error {
	blobPath, err := s.path(ctx, key)
	if err != nil {
		return err
	}

	if err := s.client.Remove(blobPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete SSH file %q: %w", blobPath, err)
	}

	return nil
}

func (s BlobStore) DeletePrefix(ctx context.Context, prefix string) error {
	prefixPath, err := s.path(ctx, prefix)
	if err != nil {
		return err
	}

	if err := s.client.RemoveAll(prefixPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete SSH directory %q: %w", prefixPath, err)
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

	return path.Join(s.root, key), nil
}

func NewBlobStore(client *sftp.Client, repositoryPath string) *BlobStore {
	return &BlobStore{client: client, root: path.Clean(repositoryPath)}
}
