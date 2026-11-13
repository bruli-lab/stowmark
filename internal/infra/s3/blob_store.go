package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
)

type BlobStore struct {
	client *s3.Client
	bucket string
	root   string
}

func (s BlobStore) Put(ctx context.Context, key string, write func(io.Writer) error) error {
	objectKey, err := s.objectKey(ctx, key)
	if err != nil {
		return err
	}

	temporary, err := os.CreateTemp("", "stowmark-s3-object-*")
	if err != nil {
		return fmt.Errorf("create temporary object file: %w", err)
	}
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporary.Name())
	}()

	if err := write(temporary); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	size, err := temporary.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("measure temporary object %q: %w", temporary.Name(), err)
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind temporary object %q: %w", temporary.Name(), err)
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(objectKey),
		Body:          temporary,
		ContentLength: aws.Int64(size),
		IfNoneMatch:   aws.String("*"),
	})
	if err != nil {
		if isPreconditionFailedError(err) {
			return nil
		}
		return fmt.Errorf("put object %q in bucket %q: %w", objectKey, s.bucket, err)
	}

	return nil
}

func (s BlobStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	objectKey, err := s.objectKey(ctx, key)
	if err != nil {
		return nil, err
	}

	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("%w: %q", blob.ErrNotFound, key)
		}
		return nil, fmt.Errorf("get object %q from bucket %q: %w", objectKey, s.bucket, err)
	}

	return output.Body, nil
}

func (s BlobStore) Exists(ctx context.Context, key string) (bool, error) {
	objectKey, err := s.objectKey(ctx, key)
	if err != nil {
		return false, err
	}

	_, err = s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	switch {
	case err == nil:
		return true, nil
	case isNotFoundError(err):
		return false, nil
	default:
		return false, fmt.Errorf("head object %q in bucket %q: %w", objectKey, s.bucket, err)
	}
}

func (s BlobStore) List(ctx context.Context, prefix string) ([]string, error) {
	objectPrefix, err := s.objectKey(ctx, prefix)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0)
	err = s.each(ctx, objectPrefix+"/", func(objectKey string) error {
		keys = append(keys, s.blobKey(objectKey))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)

	return keys, nil
}

func (s BlobStore) DeletePrefix(ctx context.Context, prefix string) error {
	objectPrefix, err := s.objectKey(ctx, prefix)
	if err != nil {
		return err
	}

	return s.each(ctx, objectPrefix+"/", func(objectKey string) error {
		_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(objectKey),
		})
		if err != nil {
			return fmt.Errorf("delete object %q from bucket %q: %w", objectKey, s.bucket, err)
		}
		return nil
	})
}

func (s BlobStore) each(ctx context.Context, objectPrefix string, visit func(objectKey string) error) error {
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(objectPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list objects under %q in bucket %q: %w", objectPrefix, s.bucket, err)
		}
		for _, item := range page.Contents {
			objectKey := aws.ToString(item.Key)
			if objectKey == "" {
				continue
			}
			if err := visit(objectKey); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s BlobStore) objectKey(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := blob.ValidateKey(key); err != nil {
		return "", err
	}

	return path.Join(s.root, key), nil
}

func (s BlobStore) blobKey(objectKey string) string {
	if s.root == "" {
		return objectKey
	}
	return strings.TrimPrefix(objectKey, s.root+"/")
}

func NewBlobStore(client *s3.Client, bucket, prefix string) *BlobStore {
	return &BlobStore{client: client, bucket: bucket, root: strings.Trim(prefix, "/")}
}

func isPreconditionFailedError(err error) bool {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		return apiErr.ErrorCode() == "PreconditionFailed"
	}
	return false
}

func isNotFoundError(err error) bool {
	if _, ok := errors.AsType[*types.NoSuchKey](err); ok {
		return true
	}
	if _, ok := errors.AsType[*types.NotFound](err); ok {
		return true
	}
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound":
			return true
		}
	}
	return false
}
