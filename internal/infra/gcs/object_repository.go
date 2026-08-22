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

	objectPath := path.Join(directory, hash[:2], hash)

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
	objectPath := dest.ObjectPath

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

	output := io.Writer(writer)
	var closeEncryption func() error

	if len(key) > 0 {
		encrypted, err := o.encryptionHandler.Encode(writer, key)
		if err != nil {
			abortUpload()

			return fmt.Errorf("create encryption writer for object %q: %w", objectPath, err)
		}

		output = encrypted.Writer
		closeEncryption = encrypted.Closer
	}

	compressionDestination := io.MultiWriter(
		output,
		hasher,
	)

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		abortUpload()

		return fmt.Errorf("get compression handler %q: %w", comp.CompType(), err)
	}

	encoder, err := handler.Encode(
		compressionDestination,
		comp.Level(),
	)
	if err != nil {
		abortUpload()

		return fmt.Errorf("create compression encoder for chunk %q: %w", hash, err)
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

		return fmt.Errorf("write chunk %q from %q at offset %d: %w", hash, filePath, offset, copyErr)
	}

	if encoder.Closer != nil {
		if err := encoder.Closer(); err != nil {
			abortUpload()

			return fmt.Errorf("finish compression for chunk %q: %w", hash, err)
		}
	}

	calculatedHash := hex.EncodeToString(hasher.Sum(nil))

	if calculatedHash != hash {
		abortUpload()

		return fmt.Errorf("chunk hash mismatch: expected %s, calculated %s", hash, calculatedHash)
	}

	if closeEncryption != nil {
		if err := closeEncryption(); err != nil {
			abortUpload()

			return fmt.Errorf("finish encryption for chunk %q: %w", hash, err)
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close object %q in bucket %q: %w", objectPath, o.bucket, err)
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

	output := io.Writer(destination)
	var closeEncryption func() error

	if len(symmetricKey) > 0 {
		encrypted, err := o.encryptionHandler.Encode(destination, symmetricKey)
		if err != nil {
			return fmt.Errorf("create encryption writer: %w", err)
		}

		output = encrypted.Writer
		closeEncryption = encrypted.Closer
	}

	encoded, err := handler.Encode(output, comp.Level())
	if err != nil {
		if closeEncryption != nil {
			_ = closeEncryption()
		}
		cancel()

		return fmt.Errorf("create encoder for object %q: %w", objectPath, err)
	}

	if _, err := io.Copy(encoded.Writer, source); err != nil {
		cancel()

		if encoded.Closer != nil {
			_ = encoded.Closer()
			if closeEncryption != nil {
				_ = closeEncryption()
			}
		}

		return fmt.Errorf("copy object %q: %w", objectPath, err)
	}

	if encoded.Closer != nil {
		if err := encoded.Closer(); err != nil {
			cancel()

			return fmt.Errorf("close encoder for object %q: %w", objectPath, err)
		}
	}

	if closeEncryption != nil {
		_ = closeEncryption()
	}

	if err := destination.Close(); err != nil {
		if isPreconditionFailed(err) {
			return nil
		}

		return fmt.Errorf("close object %q in bucket %q: %w", objectPath, o.bucket, err)
	}

	return nil
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

		written, err := o.restoreObjectPart(ctx, comp, hash, destination, symmetricKey, generation)
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

	reader := io.Reader(source)

	if len(symmetricKey) > 0 {
		reader, err = o.encryptionHandler.Decode(source, symmetricKey)
		if err != nil {
			return 0, fmt.Errorf("decrypt object %q: %w", objectPath, err)
		}
	}

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return 0, fmt.Errorf("get compression handler %q: %w", comp.CompType(), err)
	}

	decoded, err := handler.Decode(reader)
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
	return &ObjectRepository{
		repositoryPath:    repositoryPath,
		bucket:            bucket,
		client:            client,
		handlersFactory:   compression.NewHandlersFactory(),
		encryptionHandler: encrypt.NewAESGCMHandler(),
	}
}

func isPreconditionFailed(err error) bool {
	var apiErr *googleapi.Error

	return errors.As(err, &apiErr) &&
		apiErr.Code == http.StatusPreconditionFailed
}
