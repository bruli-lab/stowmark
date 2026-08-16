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
	"github.com/bruli-lab/stowmark/internal/infra/chunkio"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/bruli-lab/stowmark/internal/infra/object"
	"google.golang.org/api/googleapi"
)

type ObjectRepository struct {
	client          *storage.Client
	bucket          string
	repositoryPath  string
	handlersFactory *compression.HandlersFactory
}

func (o ObjectRepository) SaveChunk(ctx context.Context, filePath, hash string, offset, size int64, comp *repository.Compression) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	source, err := chunkio.OpenSource(filePath, hash, offset, size, comp)
	if err != nil {
		return err
	}
	defer func() {
		_ = source.Close()
	}()

	objectPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	uploadCtx, cancelUpload := context.WithCancel(ctx)
	defer cancelUpload()

	writer := o.client.
		Bucket(o.bucket).
		Object(objectPath).
		NewWriter(uploadCtx)

	abortUpload := func() {
		cancelUpload()
		_ = writer.Close()
	}

	hasher := sha256.New()

	destination := io.MultiWriter(
		writer,
		hasher,
	)

	handler, err := o.handlersFactory.GetHandler(
		comp.CompType(),
	)
	if err != nil {
		abortUpload()
		return err
	}

	encoder, err := handler.Encode(
		destination,
		comp.Level(),
	)
	if err != nil {
		abortUpload()

		return fmt.Errorf(
			"create compression encoder for chunk %q: %w",
			hash,
			err,
		)
	}

	section := io.NewSectionReader(
		source,
		offset,
		size,
	)

	_, copyErr := io.Copy(
		encoder.Writer,
		model.ContextReader{
			Ctx:    uploadCtx,
			Reader: section,
		},
	)
	if copyErr != nil {
		cancelUpload()

		if encoder.Closer != nil {
			_ = encoder.Closer()
		}

		_ = writer.Close()

		return fmt.Errorf(
			"write chunk %q from %q at offset %d: %w",
			hash,
			filePath,
			offset,
			copyErr,
		)
	}

	if encoder.Closer != nil {
		if err := encoder.Closer(); err != nil {
			abortUpload()

			return fmt.Errorf(
				"finish compression for chunk %q: %w",
				hash,
				err,
			)
		}
	}

	calculatedHash := hex.EncodeToString(
		hasher.Sum(nil),
	)

	if calculatedHash != hash {
		abortUpload()

		return fmt.Errorf(
			"chunk hash mismatch: expected %s, calculated %s",
			hash,
			calculatedHash,
		)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf(
			"close object %q in bucket %q: %w",
			objectPath,
			o.bucket,
			err,
		)
	}

	return nil
}

func (o ObjectRepository) Save(ctx context.Context, filePath, hash string, comp *repository.Compression) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if comp == nil {
		return errors.New("compression configuration is required")
	}

	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	objectPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	obj := o.client.
		Bucket(o.bucket).
		Object(objectPath)

	if _, err := obj.Attrs(ctx); err == nil {
		return nil
	} else if !errors.Is(err, storage.ErrObjectNotExist) {
		return fmt.Errorf("get attributes for object %q in bucket %q: %w", objectPath, o.bucket, err)
	}

	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", filePath, err)
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

	destination := obj.
		If(storage.Conditions{
			DoesNotExist: true,
		}).
		NewWriter(writeCtx)

	destination.ContentType = "application/octet-stream"

	encoded, err := handler.Encode(destination, comp.Level())
	if err != nil {
		cancel()

		return fmt.Errorf("create encoder for object %q: %w", objectPath, err)
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

			return fmt.Errorf("close encoder for object %q: %w", objectPath, err)
		}
	}

	if err := destination.Close(); err != nil {
		if isPreconditionFailed(err) {
			return nil
		}

		return fmt.Errorf("close object %q in bucket %q: %w", objectPath, o.bucket, err)
	}

	return nil
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, hash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

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

func (o ObjectRepository) ReadObject(ctx context.Context, hash string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf(
			"invalid object hash %q",
			hash,
		)
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
			return nil, snapshot.NewNotFoundError(hash)
		}

		return nil, fmt.Errorf(
			"open object %q in bucket %q: %w",
			objectPath,
			o.bucket,
			err,
		)
	}

	return reader, nil
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

	hashes, err := object.GetHashes(obj)
	if err != nil {
		return err
	}

	destinationPath := obj.Path()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return fmt.Errorf("create destination directory for %q: %w", destinationPath, err)
	}

	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create restored file %q: %w", destinationPath, err)
	}

	restoreCompleted := false
	destinationClosed := false

	defer func() {
		if !destinationClosed {
			_ = destination.Close()
		}

		if !restoreCompleted {
			_ = os.Remove(destinationPath)
		}
	}()

	var restoredSize int64

	for index, hash := range hashes {
		if err := ctx.Err(); err != nil {
			return err
		}

		written, err := o.restoreObjectPart(ctx, comp, hash, destination)
		if err != nil {
			return fmt.Errorf(
				"restore GCS part %d/%d of %q: %w",
				index+1,
				len(hashes),
				destinationPath,
				err,
			)
		}

		restoredSize += written
	}

	if restoredSize != obj.Size() {
		return fmt.Errorf(
			"restored size mismatch for %q: expected %d, restored %d",
			destinationPath,
			obj.Size(),
			restoredSize,
		)
	}

	if err := destination.Close(); err != nil {
		destinationClosed = true
		return fmt.Errorf("close restored file %q: %w", destinationPath, err)
	}

	destinationClosed = true
	restoreCompleted = true

	return nil
}

func (o ObjectRepository) restoreObjectPart(ctx context.Context, comp *repository.Compression, hash string, destination io.Writer) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	if len(hash) < 3 {
		return 0, fmt.Errorf("invalid object hash %q", hash)
	}

	objectPath := path.Join(o.repositoryPath, repository.ObjectsFolder, hash[:2], hash[2:])

	source, err := o.client.Bucket(o.bucket).Object(objectPath).NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return 0, snapshot.NewNotFoundError(hash)
		}

		return 0, fmt.Errorf("open object %q in bucket %q: %w", objectPath, o.bucket, err)
	}
	defer func() {
		_ = source.Close()
	}()

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return 0, fmt.Errorf("get compression handler %q: %w", comp.CompType(), err)
	}

	decoded, err := handler.Decode(source)
	if err != nil {
		return 0, fmt.Errorf("decode object %q using %q: %w", objectPath, comp.CompType(), err)
	}

	if decoded.Closer != nil {
		defer decoded.Closer()
	}

	written, err := io.Copy(destination, model.ContextReader{
		Ctx:    ctx,
		Reader: decoded.Reader,
	})
	if err != nil {
		return written, fmt.Errorf("copy decoded object %q from bucket %q: %w", objectPath, o.bucket, err)
	}

	return written, nil
}

func NewObjectRepository(repositoryPath, bucket string, client *storage.Client) *ObjectRepository {
	return &ObjectRepository{repositoryPath: repositoryPath, bucket: bucket, client: client, handlersFactory: compression.NewHandlersFactory()}
}

func isPreconditionFailed(err error) bool {
	var apiErr *googleapi.Error

	return errors.As(err, &apiErr) &&
		apiErr.Code == http.StatusPreconditionFailed
}
