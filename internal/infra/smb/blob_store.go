package smb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"

	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/cloudsoda/go-smb2"
)

const smbDirectoryMode os.FileMode = 0o755

type BlobStore struct {
	share *smb2.Share
	root  string
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

	share := s.share.WithContext(ctx)
	directory := path.Dir(blobPath)
	if err := share.MkdirAll(directory, smbDirectoryMode); err != nil {
		return fmt.Errorf("create SMB directory %q: %w", directory, err)
	}

	return writeFileAtomically(share, blobPath, func(destination io.Writer) error {
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

	file, err := s.share.WithContext(ctx).Open(blobPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %q", blob.ErrNotFound, key)
		}
		return nil, fmt.Errorf("open SMB file %q: %w", blobPath, err)
	}

	return file, nil
}

func (s BlobStore) Exists(ctx context.Context, key string) (bool, error) {
	blobPath, err := s.path(ctx, key)
	if err != nil {
		return false, err
	}

	_, err = s.share.WithContext(ctx).Stat(blobPath)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("stat SMB file %q: %w", blobPath, err)
	}
}

func (s BlobStore) List(ctx context.Context, prefix string) ([]string, error) {
	prefixPath, err := s.path(ctx, prefix)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0)
	if err := s.walk(ctx, s.share.WithContext(ctx), prefixPath, prefix, &keys); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	sort.Strings(keys)

	return keys, nil
}

func (s BlobStore) walk(ctx context.Context, share *smb2.Share, directory, keyPrefix string, keys *[]string) error {
	entries, err := share.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("list SMB directory %q: %w", directory, err)
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if isTemporaryFile(entry.Name()) {
			continue
		}
		entryPath := path.Join(directory, entry.Name())
		entryKey := path.Join(keyPrefix, entry.Name())
		if entry.IsDir() {
			if err := s.walk(ctx, share, entryPath, entryKey, keys); err != nil {
				return err
			}
			continue
		}
		*keys = append(*keys, entryKey)
	}

	return nil
}

func (s BlobStore) Delete(ctx context.Context, key string) error {
	blobPath, err := s.path(ctx, key)
	if err != nil {
		return err
	}

	if err := s.share.WithContext(ctx).Remove(blobPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete SMB file %q: %w", blobPath, err)
	}

	return nil
}

func (s BlobStore) DeletePrefix(ctx context.Context, prefix string) error {
	prefixPath, err := s.path(ctx, prefix)
	if err != nil {
		return err
	}

	if err := s.share.WithContext(ctx).RemoveAll(prefixPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete SMB directory %q: %w", prefixPath, err)
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

func NewBlobStore(share *smb2.Share, repositoryPath string) *BlobStore {
	return &BlobStore{share: share, root: repositoryPath}
}

func (s BlobStore) DeleteTemporaries(ctx context.Context, prefix string) (int, error) {
	prefixPath, err := s.path(ctx, prefix)
	if err != nil {
		return 0, err
	}

	removed := 0
	if err := s.deleteTemporaries(ctx, s.share.WithContext(ctx), prefixPath, &removed); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return removed, nil
		}
		return removed, err
	}

	return removed, nil
}

func (s BlobStore) deleteTemporaries(ctx context.Context, share *smb2.Share, directory string, removed *int) error {
	entries, err := share.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("list SMB directory %q: %w", directory, err)
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		entryPath := path.Join(directory, entry.Name())
		if entry.IsDir() {
			if err := s.deleteTemporaries(ctx, share, entryPath, removed); err != nil {
				return err
			}
			continue
		}
		if !isTemporaryFile(entry.Name()) {
			continue
		}
		if err := share.Remove(entryPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("delete SMB temporary file %q: %w", entryPath, err)
		}
		*removed++
	}

	return nil
}
