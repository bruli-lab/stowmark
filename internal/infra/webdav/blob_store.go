package webdav

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/studio-b12/gowebdav"
)

const (
	temporaryFilePrefix = "."
	webDAVFileMode      = 0o644
	webDAVDirectoryMode = 0o755
)

type BlobStore struct {
	client             *gowebdav.Client
	root               string
	mu                 *sync.Mutex
	createdDirectories map[string]struct{}
}

func (s *BlobStore) Put(ctx context.Context, key string, write func(io.Writer) error) error {
	blobPath, err := s.path(ctx, key)
	if err != nil {
		return err
	}

	exists, err := s.Exists(ctx, key)
	if err != nil || exists {
		return err
	}

	var buffer bytes.Buffer
	if err := write(&buffer); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := s.ensureDirectory(path.Dir(blobPath)); err != nil {
		return err
	}

	temporaryPath, err := temporaryFilePath(blobPath)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.client.Remove(temporaryPath)
		}
	}()

	size := int64(buffer.Len())
	if err := s.client.WriteStream(temporaryPath, &buffer, webDAVFileMode); err != nil {
		return fmt.Errorf("write WebDAV temporary file %q: %w", temporaryPath, err)
	}

	info, err := s.client.Stat(temporaryPath)
	if err != nil {
		return fmt.Errorf("stat WebDAV temporary file %q: %w", temporaryPath, err)
	}
	if info.Size() != size {
		return fmt.Errorf("WebDAV temporary file %q size mismatch: expected %d, got %d", temporaryPath, size, info.Size())
	}

	if err := s.client.Rename(temporaryPath, blobPath, false); err != nil {
		if exists, existsErr := s.Exists(ctx, key); existsErr == nil && exists {
			return nil
		}
		return fmt.Errorf("commit WebDAV file %q: %w", blobPath, err)
	}
	committed = true

	return nil
}

func (s *BlobStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	blobPath, err := s.path(ctx, key)
	if err != nil {
		return nil, err
	}

	reader, err := s.client.ReadStream(blobPath)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, fmt.Errorf("%w: %q", blob.ErrNotFound, key)
		}
		return nil, fmt.Errorf("open WebDAV file %q: %w", blobPath, err)
	}

	return reader, nil
}

func (s *BlobStore) Exists(ctx context.Context, key string) (bool, error) {
	blobPath, err := s.path(ctx, key)
	if err != nil {
		return false, err
	}

	info, err := s.client.Stat(blobPath)
	switch {
	case err == nil:
		return !info.IsDir(), nil
	case gowebdav.IsErrNotFound(err):
		return false, nil
	default:
		return false, fmt.Errorf("stat WebDAV file %q: %w", blobPath, err)
	}
}

func (s *BlobStore) List(ctx context.Context, prefix string) ([]string, error) {
	prefixPath, err := s.path(ctx, prefix)
	if err != nil {
		return nil, err
	}

	if _, err := s.client.Stat(prefixPath); err != nil {
		if gowebdav.IsErrNotFound(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("stat WebDAV directory %q: %w", prefixPath, err)
	}

	keys := make([]string, 0)
	if err := s.walk(ctx, prefixPath, prefix, &keys); err != nil {
		return nil, err
	}
	sort.Strings(keys)

	return keys, nil
}

func (s *BlobStore) walk(ctx context.Context, directory, keyPrefix string, keys *[]string) error {
	entries, err := s.client.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("list WebDAV directory %q: %w", directory, err)
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), temporaryFilePrefix) {
			continue
		}
		entryKey := path.Join(keyPrefix, entry.Name())
		if entry.IsDir() {
			if err := s.walk(ctx, path.Join(directory, entry.Name()), entryKey, keys); err != nil {
				return err
			}
			continue
		}
		*keys = append(*keys, entryKey)
	}

	return nil
}

func (s *BlobStore) Delete(ctx context.Context, key string) error {
	blobPath, err := s.path(ctx, key)
	if err != nil {
		return err
	}

	if err := s.client.Remove(blobPath); err != nil && !gowebdav.IsErrNotFound(err) {
		return fmt.Errorf("delete WebDAV file %q: %w", blobPath, err)
	}

	return nil
}

func (s *BlobStore) DeletePrefix(ctx context.Context, prefix string) error {
	prefixPath, err := s.path(ctx, prefix)
	if err != nil {
		return err
	}

	if err := s.client.RemoveAll(prefixPath); err != nil && !gowebdav.IsErrNotFound(err) {
		return fmt.Errorf("delete WebDAV directory %q: %w", prefixPath, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for directory := range s.createdDirectories {
		if directory == prefixPath || strings.HasPrefix(directory, prefixPath+"/") {
			delete(s.createdDirectories, directory)
		}
	}

	return nil
}

func (s *BlobStore) ensureDirectory(directory string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.createdDirectories[directory]; ok {
		return nil
	}

	if err := s.client.MkdirAll(directory, webDAVDirectoryMode); err != nil {
		return fmt.Errorf("create WebDAV directory %q: %w", directory, err)
	}
	s.createdDirectories[directory] = struct{}{}

	return nil
}

func (s *BlobStore) path(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := blob.ValidateKey(key); err != nil {
		return "", err
	}

	return path.Join(s.root, key), nil
}

func temporaryFilePath(filePath string) (string, error) {
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("generate temporary file name: %w", err)
	}

	directory, name := path.Split(filePath)

	return path.Join(directory, temporaryFilePrefix+name+".tmp-"+hex.EncodeToString(suffix)), nil
}

func NewBlobStore(client *gowebdav.Client, repositoryPath string) *BlobStore {
	return &BlobStore{
		client:             client,
		root:               repositoryPath,
		mu:                 new(sync.Mutex),
		createdDirectories: make(map[string]struct{}),
	}
}
