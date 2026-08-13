package gcs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"cloud.google.com/go/storage"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"google.golang.org/api/googleapi"
)

type ObjectRepository struct {
	client          *storage.Client
	bucket          string
	repositoryPath  string
	handlersFactory *compression.HandlersFactory
}

func (o ObjectRepository) Save(ctx context.Context, obj *snapshot.File, comp *repository.Compression) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if obj == nil {
		return errors.New("snapshot object is required")
	}

	if comp == nil {
		return errors.New("compression configuration is required")
	}

	hash := obj.Hash()
	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	objectPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	object := o.client.
		Bucket(o.bucket).
		Object(objectPath)

	if _, err := object.Attrs(ctx); err == nil {
		return nil
	} else if !errors.Is(err, storage.ErrObjectNotExist) {
		return fmt.Errorf(
			"get attributes for object %q in bucket %q: %w",
			objectPath,
			o.bucket,
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

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return err
	}

	writeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	destination := object.
		If(storage.Conditions{
			DoesNotExist: true,
		}).
		NewWriter(writeCtx)

	destination.ContentType = "application/octet-stream"

	encoded, err := handler.Encode(destination, comp.Level())
	if err != nil {
		cancel()

		return fmt.Errorf(
			"create encoder for object %q: %w",
			objectPath,
			err,
		)
	}

	if _, err := io.Copy(encoded.Writer, source); err != nil {
		cancel()

		if encoded.Closer != nil {
			_ = encoded.Closer()
		}

		return fmt.Errorf(
			"copy object %q: %w",
			objectPath,
			err,
		)
	}

	if encoded.Closer != nil {
		if err := encoded.Closer(); err != nil {
			cancel()

			return fmt.Errorf(
				"close encoder for object %q: %w",
				objectPath,
				err,
			)
		}
	}

	if err := destination.Close(); err != nil {
		if isPreconditionFailed(err) {
			return nil
		}

		return fmt.Errorf(
			"close object %q in bucket %q: %w",
			objectPath,
			o.bucket,
			err,
		)
	}

	return nil
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, obj *snapshot.File) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if obj == nil {
		return false, errors.New("snapshot object is required")
	}

	hash := obj.Hash()
	if len(hash) < 3 {
		return false, fmt.Errorf("invalid object hash %q", hash)
	}

	objectPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	_, err := o.client.
		Bucket(o.bucket).
		Object(objectPath).
		Attrs(ctx)

	switch {
	case err == nil:
		return true, nil

	case errors.Is(err, storage.ErrObjectNotExist):
		return false, nil

	default:
		return false, fmt.Errorf("check object %q in bucket %q: %w", objectPath, o.bucket, err)
	}
}

func (o ObjectRepository) ReadObject(ctx context.Context, originalPath, hash string) (*snapshot.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	objectPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	reader, err := o.client.
		Bucket(o.bucket).
		Object(objectPath).
		NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, snapshot.NewNotFoundError(originalPath)
		}

		return nil, fmt.Errorf("open object %q in bucket %q: %w", objectPath, o.bucket, err)
	}

	hasher := sha256.New()

	storedSize, readErr := io.Copy(
		hasher,
		model.ContextReader{
			Ctx:    ctx,
			Reader: reader,
		},
	)

	closeErr := reader.Close()

	if readErr != nil {
		return nil, fmt.Errorf("read object %q in bucket %q: %w", objectPath, o.bucket, readErr)
	}

	if closeErr != nil {
		return nil, fmt.Errorf("close object %q in bucket %q: %w", objectPath, o.bucket, closeErr)
	}

	calculatedHash := hex.EncodeToString(hasher.Sum(nil))

	result := snapshot.File{}
	result.Hydrate(
		originalPath,
		calculatedHash,
		storedSize,
	)

	return &result, nil
}

func (o ObjectRepository) RestoreObject(ctx context.Context, comp *repository.Compression, obj *snapshot.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if comp == nil {
		return errors.New("compression configuration is required")
	}

	if obj == nil {
		return errors.New("snapshot object is required")
	}

	hash := obj.Hash()
	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	objectPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	source, err := o.client.
		Bucket(o.bucket).
		Object(objectPath).
		NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return snapshot.NewNotFoundError(obj.Path())
		}

		return fmt.Errorf("open object %q in bucket %q: %w", objectPath, o.bucket, err)
	}

	defer func() {
		_ = source.Close()
	}()

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return err
	}

	decoded, err := handler.Decode(source)
	if err != nil {
		return fmt.Errorf("decode object %q using %q: %w", objectPath, comp.CompType(), err)
	}

	if decoded.Closer != nil {
		defer decoded.Closer()
	}

	destinationPath := obj.Path()

	if err := os.MkdirAll(
		filepath.Dir(destinationPath),
		0o755,
	); err != nil {
		return fmt.Errorf("create destination directory for %q: %w", destinationPath, err)
	}

	destination, err := os.OpenFile(
		destinationPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0o644,
	)
	if err != nil {
		return fmt.Errorf("create restored file %q: %w", destinationPath, err)
	}

	restoreCompleted := false

	defer func() {
		if restoreCompleted {
			return
		}

		_ = destination.Close()
		_ = os.Remove(destinationPath)
	}()

	if _, err := io.Copy(
		destination,
		model.ContextReader{
			Ctx:    ctx,
			Reader: decoded.Reader,
		},
	); err != nil {
		return fmt.Errorf("restore object %q from bucket %q to %q: %w", objectPath, o.bucket, destinationPath, err)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close restored file %q: %w", destinationPath, err)
	}

	restoreCompleted = true

	return nil
}

func NewObjectRepository(repositoryPath, bucket string, client *storage.Client) *ObjectRepository {
	return &ObjectRepository{repositoryPath: repositoryPath, bucket: bucket, client: client, handlersFactory: compression.NewHandlersFactory()}
}

func isPreconditionFailed(err error) bool {
	var apiErr *googleapi.Error

	return errors.As(err, &apiErr) &&
		apiErr.Code == http.StatusPreconditionFailed
}
