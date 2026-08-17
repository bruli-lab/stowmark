package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/chunkio"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/bruli-lab/stowmark/internal/infra/object"
)

type ObjectRepository struct {
	client            *s3.Client
	bucket            string
	repositoryPath    string
	handlersFactory   *compression.HandlersFactory
	encoder           *object.Encoder
	encryptionHandler *encrypt.AESGCMHandler
}

func (o ObjectRepository) ListEncryptedObjects(ctx context.Context, generation uint64) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := object.EncryptedGenerationPath(o.repositoryPath, generation, true)

	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	paginator := s3.NewListObjectsV2Paginator(
		o.client,
		&s3.ListObjectsV2Input{
			Bucket: aws.String(o.bucket),
			Prefix: aws.String(prefix),
		},
	)

	var hashes []string

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list encrypted objects for generation %d in bucket %q: %w", generation, o.bucket, err)
		}

		for _, item := range page.Contents {
			key := aws.ToString(item.Key)
			relativeName := strings.TrimPrefix(key, prefix)
			parts := strings.Split(relativeName, "/")

			if len(parts) != 2 ||
				len(parts[0]) != 2 ||
				parts[1] == "" {
				continue
			}

			hashes = append(
				hashes,
				parts[0]+parts[1],
			)
		}
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

	objectPath := path.Join(
		directory,
		hash[:2],
		hash[2:],
	)

	result, err := o.client.GetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(o.bucket),
			Key:    aws.String(objectPath),
		},
	)
	if err != nil {
		if _, ok := errors.AsType[*types.NoSuchKey](err); ok {
			return nil, fmt.Errorf("encrypted object %q from generation %d: %w", hash, generation, os.ErrNotExist)
		}

		var apiErr smithy.APIError
		if errors.As(err, &apiErr) &&
			(apiErr.ErrorCode() == "NoSuchKey" ||
				apiErr.ErrorCode() == "NotFound") {
			return nil, fmt.Errorf("encrypted object %q from generation %d: %w", hash, generation, os.ErrNotExist)
		}

		return nil, fmt.Errorf("open encrypted object %q in bucket %q: %w", objectPath, o.bucket, err)
	}

	decoded, err := o.encryptionHandler.Decode(
		result.Body,
		key,
	)
	if err != nil {
		_ = result.Body.Close()

		return nil, fmt.Errorf("decrypt object %q from generation %d: %w", hash, generation, err)
	}

	return &model.ReadCloser{
		Reader: decoded,
		Closer: result.Body,
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

	directory := object.EncryptedGenerationPath(o.repositoryPath, generation, true)

	objectPath := path.Join(
		directory,
		hash[:2],
		hash[2:],
	)

	reader, writer := io.Pipe()
	encodeResult := make(chan error, 1)

	go func() {
		encoder, err := o.encryptionHandler.Encode(writer, key)
		if err != nil {
			_ = writer.CloseWithError(err)

			encodeResult <- fmt.Errorf("create encryption encoder for object %q: %w", hash, err)

			return
		}

		if _, err := io.Copy(encoder.Writer, source); err != nil {
			_ = encoder.Closer()
			_ = writer.CloseWithError(err)

			encodeResult <- fmt.Errorf("encrypt rekeyed object %q: %w", hash, err)

			return
		}

		if err := encoder.Closer(); err != nil {
			_ = writer.CloseWithError(err)

			encodeResult <- fmt.Errorf("finalize encryption of rekeyed object %q: %w", hash, err)

			return
		}

		if err := writer.Close(); err != nil {
			encodeResult <- fmt.Errorf("close encrypted stream for object %q: %w", hash, err)

			return
		}

		encodeResult <- nil
	}()

	transferManager := transfermanager.New(o.client)

	_, uploadErr := transferManager.UploadObject(
		ctx,
		&transfermanager.UploadObjectInput{
			Bucket: aws.String(o.bucket),
			Key:    aws.String(objectPath),
			Body:   reader,
		},
	)

	if uploadErr != nil {
		_ = reader.CloseWithError(uploadErr)
	} else {
		_ = reader.Close()
	}

	encodeErr := <-encodeResult

	if uploadErr != nil {
		return fmt.Errorf("save rekeyed object %q as %q in bucket %q: %w", hash, objectPath, o.bucket, uploadErr)
	}

	if encodeErr != nil {
		return encodeErr
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

	paginator := s3.NewListObjectsV2Paginator(
		o.client,
		&s3.ListObjectsV2Input{
			Bucket: aws.String(o.bucket),
			Prefix: aws.String(prefix),
		},
	)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list encrypted generation %d in bucket %q: %w", generation, o.bucket, err)
		}

		for _, item := range page.Contents {
			objectPath := aws.ToString(item.Key)
			if objectPath == "" {
				continue
			}

			_, err := o.client.DeleteObject(
				ctx,
				&s3.DeleteObjectInput{
					Bucket: aws.String(o.bucket),
					Key:    aws.String(objectPath),
				},
			)
			if err != nil {
				return fmt.Errorf("delete encrypted object %q from generation %d in bucket %q: %w", objectPath, generation, o.bucket, err)
			}
		}
	}

	return nil
}

func (o ObjectRepository) AbortRekey(ctx context.Context, generation uint64) error {
	if err := o.DeleteEncryptedGeneration(ctx, generation); err != nil {
		return fmt.Errorf("abort rekey of generation %d: %w", generation, err)
	}
	return nil
}

func (o ObjectRepository) ReadObject(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (io.ReadCloser, error) {
	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)
	objectPath := dest.ObjectPath

	output, err := o.client.GetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(o.bucket),
			Key:    aws.String(objectPath),
		},
	)
	if err != nil {
		if isObjectNotFound(err) {
			return nil, snapshot.NewNotFoundError(hash)
		}

		return nil, fmt.Errorf("open object %q in bucket %q: %w", objectPath, o.bucket, err)
	}

	reader := output.Body
	if reader == nil {
		return nil, fmt.Errorf("object %q in bucket %q has no response body", objectPath, o.bucket)
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

func (o ObjectRepository) SaveChunk(ctx context.Context, filePath, hash string, offset, size int64, comp *repository.Compression, key []byte, generation uint64) error {
	source, err := chunkio.OpenSource(filePath, hash, offset, size, comp)
	if err != nil {
		return err
	}
	defer func() {
		_ = source.Close()
	}()

	encoded, err := o.encoder.Encode(ctx, filePath, hash, offset, size, comp, key, source)
	if err != nil {
		return err
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, key, true)
	objectPath := dest.ObjectPath

	body := bytes.NewReader(encoded.Bytes())

	_, err = o.client.PutObject(
		ctx,
		&s3.PutObjectInput{
			Bucket:        aws.String(o.bucket),
			Key:           aws.String(objectPath),
			Body:          body,
			ContentLength: aws.Int64(int64(body.Len())),
		},
	)
	if err != nil {
		return fmt.Errorf("write object %q in bucket %q: %w", objectPath, o.bucket, err)
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
	key := dest.ObjectPath
	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", filePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	temporary, err := os.CreateTemp(
		"",
		"stowmark-s3-object-*",
	)
	if err != nil {
		return fmt.Errorf("create temporary object file: %w", err)
	}

	temporaryPath := temporary.Name()

	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()

	output := io.Writer(temporary)
	var closeEncryption func() error

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return err
	}

	if len(symmetricKey) > 0 {
		encrypted, err := o.encryptionHandler.Encode(temporary, symmetricKey)
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
		return fmt.Errorf("encode object %q using %q: %w", filePath, comp.CompType(), err)
	}

	_, copyErr := io.Copy(
		encoded.Writer,
		model.ContextReader{
			Ctx:    ctx,
			Reader: source,
		},
	)
	if copyErr != nil {
		if closeEncryption != nil {
			_ = closeEncryption()
		}
		return fmt.Errorf("encode object %q: %w", filePath, copyErr)
	}

	if encoded.Closer != nil {
		if err := encoded.Closer(); err != nil {
			return fmt.Errorf("close encoded object %q: %w", filePath, err)
		}
	}
	if closeEncryption != nil {
		_ = closeEncryption()
	}

	objectInfo, err := temporary.Stat()
	if err != nil {
		return fmt.Errorf(
			"stat temporary object %q: %w",
			temporaryPath,
			err,
		)
	}

	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind temporary object %q: %w", temporaryPath, err)
	}

	_, err = o.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(o.bucket),
		Key:           aws.String(key),
		Body:          temporary,
		ContentLength: aws.Int64(objectInfo.Size()),
		IfNoneMatch:   aws.String("*"),
	})
	if err != nil {
		if isPreconditionFailedError(err) {
			return nil
		}

		return fmt.Errorf("write object %q in bucket %q: %w", key, o.bucket, err)
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
	key := dest.ObjectPath
	_, err := o.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(o.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}

	if isNotFoundError(err) {
		return false, nil
	}

	return false, fmt.Errorf("check object %q in bucket %q: %w", key, o.bucket, err)
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
				"restore S3 part %d/%d of %q: %w",
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
	key := dest.ObjectPath

	output, err := o.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(o.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFoundError(err) {
			return 0, snapshot.NewNotFoundError(hash)
		}

		return 0, fmt.Errorf("get object %q from bucket %q: %w", key, o.bucket, err)
	}
	defer func() {
		_ = output.Body.Close()
	}()

	reader := io.Reader(output.Body)

	if len(symmetricKey) > 0 {
		reader, err = o.encryptionHandler.Decode(output.Body, symmetricKey)
		if err != nil {
			return 0, fmt.Errorf("decrypt object %q: %w", key, err)
		}
	}

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return 0, fmt.Errorf("get compression handler %q: %w", comp.CompType(), err)
	}

	decoded, err := handler.Decode(reader)
	if err != nil {
		return 0, fmt.Errorf("decode object %q using %q: %w", key, comp.CompType(), err)
	}

	if decoded.Closer != nil {
		defer decoded.Closer()
	}

	written, err := io.Copy(destination, model.ContextReader{
		Ctx:    ctx,
		Reader: decoded.Reader,
	})
	if err != nil {
		return written, fmt.Errorf("copy decoded object %q from bucket %q: %w", key, o.bucket, err)
	}

	return written, nil
}

func NewObjectRepository(client *s3.Client, bucket, repositoryPath string) *ObjectRepository {
	handlersFactory := compression.NewHandlersFactory()
	return &ObjectRepository{
		client:            client,
		bucket:            bucket,
		repositoryPath:    repositoryPath,
		handlersFactory:   handlersFactory,
		encoder:           object.NewEncoder(handlersFactory),
		encryptionHandler: encrypt.NewAESGCMHandler(),
	}
}

func isPreconditionFailedError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	return apiErr.ErrorCode() == "PreconditionFailed"
}

func isObjectNotFound(err error) bool {
	if _, ok := errors.AsType[*types.NoSuchKey](err); ok {
		return true
	}

	if responseError, ok := errors.AsType[*smithyhttp.ResponseError](err); ok {
		return responseError.HTTPStatusCode() == http.StatusNotFound
	}

	return false
}
