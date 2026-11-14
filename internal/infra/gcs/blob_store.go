package gcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

type BlobStore struct {
	client *storage.Client
	bucket string
	root   string
}

func (s BlobStore) Put(ctx context.Context, key string, write func(io.Writer) error) error {
	objectName, err := s.objectName(ctx, key)
	if err != nil {
		return err
	}

	uploadCtx, cancelUpload := context.WithCancel(ctx)
	defer cancelUpload()

	writer := s.client.Bucket(s.bucket).Object(objectName).
		If(storage.Conditions{DoesNotExist: true}).
		NewWriter(uploadCtx)
	writer.ContentType = "application/octet-stream"

	if err := write(writer); err != nil {
		cancelUpload()
		_ = writer.Close()
		return err
	}
	if err := ctx.Err(); err != nil {
		cancelUpload()
		_ = writer.Close()
		return err
	}

	if err := writer.Close(); err != nil {
		if isPreconditionFailed(err) {
			return nil
		}
		return fmt.Errorf("write object %q in bucket %q: %w", objectName, s.bucket, err)
	}

	return nil
}

func (s BlobStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	objectName, err := s.objectName(ctx, key)
	if err != nil {
		return nil, err
	}

	reader, err := s.client.Bucket(s.bucket).Object(objectName).NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, fmt.Errorf("%w: %q", blob.ErrNotFound, key)
		}
		return nil, fmt.Errorf("open object %q in bucket %q: %w", objectName, s.bucket, err)
	}

	return reader, nil
}

func (s BlobStore) Exists(ctx context.Context, key string) (bool, error) {
	objectName, err := s.objectName(ctx, key)
	if err != nil {
		return false, err
	}

	_, err = s.client.Bucket(s.bucket).Object(objectName).Attrs(ctx)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, storage.ErrObjectNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("stat object %q in bucket %q: %w", objectName, s.bucket, err)
	}
}

func (s BlobStore) List(ctx context.Context, prefix string) ([]string, error) {
	objectPrefix, err := s.objectName(ctx, prefix)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0)
	err = s.each(ctx, objectPrefix+"/", func(objectName string) error {
		keys = append(keys, strings.TrimPrefix(objectName, s.root+"/"))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)

	return keys, nil
}

func (s BlobStore) Delete(ctx context.Context, key string) error {
	objectName, err := s.objectName(ctx, key)
	if err != nil {
		return err
	}

	if err := s.client.Bucket(s.bucket).Object(objectName).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return fmt.Errorf("delete object %q from bucket %q: %w", objectName, s.bucket, err)
	}

	return nil
}

func (s BlobStore) DeletePrefix(ctx context.Context, prefix string) error {
	objectPrefix, err := s.objectName(ctx, prefix)
	if err != nil {
		return err
	}

	bucket := s.client.Bucket(s.bucket)
	return s.each(ctx, objectPrefix+"/", func(objectName string) error {
		if err := bucket.Object(objectName).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
			return fmt.Errorf("delete object %q from bucket %q: %w", objectName, s.bucket, err)
		}
		return nil
	})
}

func (s BlobStore) each(ctx context.Context, objectPrefix string, visit func(objectName string) error) error {
	objects := s.client.Bucket(s.bucket).Objects(ctx, &storage.Query{Prefix: objectPrefix})
	for {
		attrs, err := objects.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("list objects under %q in bucket %q: %w", objectPrefix, s.bucket, err)
		}
		if err := visit(attrs.Name); err != nil {
			return err
		}
	}
}

func (s BlobStore) objectName(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := blob.ValidateKey(key); err != nil {
		return "", err
	}

	return path.Join(s.root, key), nil
}

func NewBlobStore(client *storage.Client, bucket, repositoryPath string) *BlobStore {
	return &BlobStore{client: client, bucket: bucket, root: path.Clean(repositoryPath)}
}

func isPreconditionFailed(err error) bool {
	if apiErr, ok := errors.AsType[*googleapi.Error](err); ok {
		return apiErr.Code == http.StatusPreconditionFailed
	}
	return false
}
