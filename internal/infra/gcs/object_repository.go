package gcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/chunkio"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/bruli-lab/stowmark/internal/infra/object"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

type ObjectRepository struct {
	client            *storage.Client
	bucket            string
	repositoryPath    string
	handlersFactory   *compression.HandlersFactory
	encryptionHandler *encrypt.AESGCMHandler
	pipeline          *object.Pipeline
}

func (o ObjectRepository) ListEncryptedObjects(ctx context.Context, generation uint64) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := object.EncryptedGenerationPath(
		o.repositoryPath,
		generation,
		true,
	)

	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	objectsIterator := o.client.
		Bucket(o.bucket).
		Objects(ctx, &storage.Query{
			Prefix: prefix,
		})

	var hashes []string

	for {
		attrs, err := objectsIterator.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list encrypted objects for generation %d in bucket %q: %w", generation, o.bucket, err)
		}

		relativeName := strings.TrimPrefix(attrs.Name, prefix)
		parts := strings.Split(relativeName, "/")

		if len(parts) != 2 || len(parts[0]) != 2 || parts[1] == "" {
			continue
		}

		hashes = append(hashes, parts[0]+parts[1])
	}

	sort.Strings(hashes)

	return hashes, nil
}

func (o ObjectRepository) ReadEncryptedObject(ctx context.Context, hash string, generation uint64, key []byte) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	if len(key) == 0 {
		return nil, errors.New("symmetric key is required")
	}

	directory := object.EncryptedGenerationPath(o.repositoryPath, generation, true)

	objectPath := path.Join(directory, hash[:2], hash[2:])

	source, err := o.client.
		Bucket(o.bucket).
		Object(objectPath).
		NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, fmt.Errorf("encrypted object %q from generation %d: %w", hash, generation, os.ErrNotExist)
		}

		return nil, fmt.Errorf("open encrypted object %q in bucket %q: %w", objectPath, o.bucket, err)
	}

	decoded, err := o.encryptionHandler.Decode(source, key)
	if err != nil {
		_ = source.Close()

		return nil, fmt.Errorf(
			"decrypt object %q from generation %d: %w",
			hash,
			generation,
			err,
		)
	}
	return &model.ReadCloser{
		Reader: decoded,
		Closer: source,
	}, nil
}

func (o ObjectRepository) SaveRekeyedObject(ctx context.Context, hash string, source io.Reader, generation uint64, key []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	if source == nil {
		return errors.New("source is required")
	}

	if len(key) == 0 {
		return errors.New("symmetric key is required")
	}

	directory := object.EncryptedGenerationPath(
		o.repositoryPath,
		generation,
		true,
	)

	objectPath := path.Join(directory, hash[:2], hash[2:])

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

	encoder, err := o.encryptionHandler.Encode(writer, key)
	if err != nil {
		abortUpload()

		return fmt.Errorf(
			"create encryption encoder for object %q: %w",
			hash,
			err,
		)
	}

	if _, err := io.Copy(encoder.Writer, source); err != nil {
		_ = encoder.Closer()
		abortUpload()

		return fmt.Errorf("encrypt rekeyed object %q: %w", hash, err)
	}

	if err := encoder.Closer(); err != nil {
		abortUpload()

		return fmt.Errorf("finalize encryption of rekeyed object %q: %w", hash, err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf(
			"save rekeyed object %q as %q in bucket %q: %w",
			hash,
			objectPath,
			o.bucket,
			err,
		)
	}

	return nil
}

func (o ObjectRepository) DeleteEncryptedGeneration(ctx context.Context, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	prefix := object.EncryptedGenerationPath(o.repositoryPath, generation, true)

	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	bucket := o.client.Bucket(o.bucket)

	objectsIterator := bucket.Objects(ctx, &storage.Query{
		Prefix: prefix,
	})

	for {
		attrs, err := objectsIterator.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return fmt.Errorf(
				"list encrypted generation %d in bucket %q: %w",
				generation,
				o.bucket,
				err,
			)
		}

		if err := bucket.Object(attrs.Name).Delete(ctx); err != nil {
			if errors.Is(err, storage.ErrObjectNotExist) {
				continue
			}

			return fmt.Errorf(
				"delete encrypted object %q from generation %d in bucket %q: %w",
				attrs.Name,
				generation,
				o.bucket,
				err,
			)
		}
	}

	return nil
}

func (o ObjectRepository) AbortRekey(ctx context.Context, generation uint64) error {
	if err := o.DeleteEncryptedGeneration(ctx, generation); err != nil {
		return fmt.Errorf("abort rekey for generation %d: %w", generation, err)
	}
	return nil
}

func (o ObjectRepository) SaveChunk(ctx context.Context, filePath, hash string, offset, size int64, comp *repository.Compression, key []byte, generation uint64) error {
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

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, key, true)
	obj := o.client.Bucket(o.bucket).Object(dest.ObjectPath)
	section := io.NewSectionReader(source, offset, size)

	if err := o.upload(ctx, obj, section, comp, key, hash); err != nil {
		return fmt.Errorf("write chunk %q from %q at offset %d: %w", hash, filePath, offset, err)
	}

	return nil
}

func (o ObjectRepository) Save(ctx context.Context, filePath, hash string, comp *repository.Compression, symmetricKey []byte, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if comp == nil {
		return errors.New("compression configuration is required")
	}

	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)
	objectPath := dest.ObjectPath
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

	conditional := obj.If(storage.Conditions{DoesNotExist: true})
	if err := o.upload(ctx, conditional, source, comp, symmetricKey, hash); err != nil {
		if isPreconditionFailed(err) {
			return nil
		}

		return fmt.Errorf("write object %q in bucket %q: %w", objectPath, o.bucket, err)
	}

	return nil
}

func (o ObjectRepository) upload(ctx context.Context, obj *storage.ObjectHandle, source io.Reader, comp *repository.Compression, key []byte, hash string) error {
	uploadCtx, cancelUpload := context.WithCancel(ctx)
	defer cancelUpload()

	writer := obj.NewWriter(uploadCtx)
	writer.ContentType = "application/octet-stream"

	if err := o.pipeline.Write(uploadCtx, writer, source, comp, key, hash); err != nil {
		cancelUpload()
		_ = writer.Close()

		return err
	}

	return writer.Close()
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if len(hash) < 3 {
		return false, fmt.Errorf("invalid object hash %q", hash)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)
	objectPath := dest.ObjectPath
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

func (o ObjectRepository) ReadObject(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf(
			"invalid object hash %q",
			hash,
		)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)
	objectPath := dest.ObjectPath

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

	if len(symmetricKey) == 0 {
		return reader, nil
	}

	decoded, err := o.encryptionHandler.Decode(reader, symmetricKey)
	if err != nil {
		_ = reader.Close()
		return nil, fmt.Errorf("decrypt object %q: %w", dest.ObjectPath, err)
	}

	return &model.ReadCloser{
		Reader: decoded,
		Closer: reader,
	}, nil
}

func (o ObjectRepository) RestoreObject(ctx context.Context, comp *repository.Compression, obj *snapshot.File, symmetricKey []byte, generation uint64) error {
	return object.RestoreFile(ctx, comp, obj, func(ctx context.Context, hash string, destination io.Writer) (int64, error) {
		return o.restoreObjectPart(ctx, comp, hash, destination, symmetricKey, generation)
	})
}

func (o ObjectRepository) restoreObjectPart(ctx context.Context, comp *repository.Compression, hash string, destination io.Writer, symmetricKey []byte, generation uint64) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	if len(hash) < 3 {
		return 0, fmt.Errorf("invalid object hash %q", hash)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)
	objectPath := dest.ObjectPath

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

	written, err := o.pipeline.Read(ctx, destination, source, comp, symmetricKey, hash)
	if err != nil {
		return written, fmt.Errorf("restore object %q: %w", objectPath, err)
	}

	return written, nil
}

func NewObjectRepository(repositoryPath, bucket string, client *storage.Client) *ObjectRepository {
	handlersFactory := compression.NewHandlersFactory()
	return &ObjectRepository{
		repositoryPath:    repositoryPath,
		bucket:            bucket,
		client:            client,
		handlersFactory:   handlersFactory,
		encryptionHandler: encrypt.NewAESGCMHandler(),
		pipeline:          object.NewPipeline(handlersFactory),
	}
}

func isPreconditionFailed(err error) bool {
	var apiErr *googleapi.Error

	return errors.As(err, &apiErr) &&
		apiErr.Code == http.StatusPreconditionFailed
}
