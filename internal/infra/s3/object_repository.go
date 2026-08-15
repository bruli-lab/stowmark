package s3

import (
	"bytes"
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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

type ObjectRepository struct {
	client          *s3.Client
	bucket          string
	repositoryPath  string
	handlersFactory *compression.HandlersFactory
}

func (o ObjectRepository) ReadObject(ctx context.Context, hash string) (io.ReadCloser, error) {
	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	objectPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

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

	if output.Body == nil {
		return nil, fmt.Errorf("object %q in bucket %q has no response body", objectPath, o.bucket)
	}

	return output.Body, nil
}

func (o ObjectRepository) SaveChunk(ctx context.Context, filePath, hash string, offset, size int64, comp *repository.Compression) error {
	if comp == nil {
		return errors.New("compression configuration is required")
	}

	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	if offset < 0 {
		return fmt.Errorf("invalid chunk offset: %d", offset)
	}

	if size <= 0 {
		return fmt.Errorf("invalid chunk size: %d", size)
	}

	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", filePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	info, err := source.Stat()
	if err != nil {
		return fmt.Errorf("stat source file %q: %w", filePath, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file", filePath)
	}

	if offset > info.Size() || size > info.Size()-offset {
		return fmt.Errorf("chunk range [%d,%d) exceeds file size %d for %q", offset, offset+size, info.Size(), filePath)
	}

	section := io.NewSectionReader(
		source,
		offset,
		size,
	)

	var encoded bytes.Buffer

	handler, err := o.handlersFactory.GetHandler(
		comp.CompType(),
	)
	if err != nil {
		return err
	}

	encoder, err := handler.Encode(
		&encoded,
		comp.Level(),
	)
	if err != nil {
		return fmt.Errorf("create compression encoder for chunk %q: %w", hash, err)
	}

	_, copyErr := io.Copy(
		encoder.Writer,
		model.ContextReader{
			Ctx:    ctx,
			Reader: section,
		},
	)
	if copyErr != nil {
		if encoder.Closer != nil {
			_ = encoder.Closer()
		}

		return fmt.Errorf("compress chunk %q from %q at offset %d: %w", hash, filePath, offset, copyErr)
	}

	if encoder.Closer != nil {
		if err := encoder.Closer(); err != nil {
			return fmt.Errorf("finish compression for chunk %q: %w", hash, err)
		}
	}

	calculatedHashBytes := sha256.Sum256(
		encoded.Bytes(),
	)

	calculatedHash := hex.EncodeToString(
		calculatedHashBytes[:],
	)

	if calculatedHash != hash {
		return fmt.Errorf("chunk hash mismatch: expected %s, calculated %s", hash, calculatedHash)
	}

	objectPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

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

	key := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

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

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return err
	}

	encoded, err := handler.Encode(temporary, comp.Level())
	if err != nil {
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
		return fmt.Errorf("encode object %q: %w", filePath, copyErr)
	}

	if encoded.Closer != nil {
		if err := encoded.Closer(); err != nil {
			return fmt.Errorf("close encoded object %q: %w", filePath, err)
		}
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

func (o ObjectRepository) AlreadyExists(ctx context.Context, hash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if len(hash) < 3 {
		return false, fmt.Errorf("invalid object hash %q", hash)
	}

	key := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

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

	key := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	output, err := o.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(o.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFoundError(err) {
			return snapshot.NewNotFoundError(obj.Path())
		}

		return fmt.Errorf(
			"get object %q from bucket %q: %w",
			key,
			o.bucket,
			err,
		)
	}
	defer func() {
		_ = output.Body.Close()
	}()

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return err
	}

	decoded, err := handler.Decode(output.Body)
	if err != nil {
		return fmt.Errorf(
			"decode object %q using %q: %w",
			key,
			comp.CompType(),
			err,
		)
	}

	if decoded.Closer != nil {
		defer decoded.Closer()
	}

	destinationPath := obj.Path()

	if err := os.MkdirAll(
		filepath.Dir(destinationPath),
		0o755,
	); err != nil {
		return fmt.Errorf(
			"create destination directory for %q: %w",
			destinationPath,
			err,
		)
	}

	destination, err := os.OpenFile(
		destinationPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0o644,
	)
	if err != nil {
		return fmt.Errorf(
			"create restored file %q: %w",
			destinationPath,
			err,
		)
	}

	restoreCompleted := false

	defer func() {
		if !restoreCompleted {
			_ = destination.Close()
			_ = os.Remove(destinationPath)
		}
	}()

	_, err = io.Copy(
		destination,
		model.ContextReader{
			Ctx:    ctx,
			Reader: decoded.Reader,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"restore object %q from bucket %q to %q: %w",
			key,
			o.bucket,
			destinationPath,
			err,
		)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf(
			"close restored file %q: %w",
			destinationPath,
			err,
		)
	}

	restoreCompleted = true

	return nil
}

func NewObjectRepository(client *s3.Client, bucket, repositoryPath string) *ObjectRepository {
	return &ObjectRepository{
		client:          client,
		bucket:          bucket,
		repositoryPath:  repositoryPath,
		handlersFactory: compression.NewHandlersFactory(),
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
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}

	var responseError *smithyhttp.ResponseError
	if errors.As(err, &responseError) {
		return responseError.HTTPStatusCode() == http.StatusNotFound
	}

	return false
}
