package s3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
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

	key := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	source, err := os.Open(obj.Path())
	if err != nil {
		return fmt.Errorf("open source file %q: %w", obj.Path(), err)
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
		return fmt.Errorf(
			"encode object %q using %q: %w",
			obj.Path(),
			comp.CompType(),
			err,
		)
	}

	_, copyErr := io.Copy(
		encoded.Writer,
		model.ContextReader{
			Ctx:    ctx,
			Reader: source,
		},
	)
	if copyErr != nil {
		return fmt.Errorf(
			"encode object %q: %w",
			obj.Path(),
			copyErr,
		)
	}

	if encoded.Closer != nil {
		if err := encoded.Closer(); err != nil {
			return fmt.Errorf(
				"close encoded object %q: %w",
				obj.Path(),
				err,
			)
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
		return fmt.Errorf(
			"rewind temporary object %q: %w",
			temporaryPath,
			err,
		)
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

		return fmt.Errorf(
			"write object %q in bucket %q: %w",
			key,
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

	hash := obj.Hash()
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

func (o ObjectRepository) ReadObject(ctx context.Context, originalPath, hash string) (*snapshot.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
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
			return nil, snapshot.NewNotFoundError(originalPath)
		}

		return nil, fmt.Errorf(
			"get object %q from bucket %q: %w",
			key,
			o.bucket,
			err,
		)
	}

	hasher := sha256.New()

	storedSize, readErr := io.Copy(
		hasher,
		model.ContextReader{
			Ctx:    ctx,
			Reader: output.Body,
		},
	)

	closeErr := output.Body.Close()

	if readErr != nil {
		return nil, fmt.Errorf(
			"read object %q from bucket %q: %w",
			key,
			o.bucket,
			readErr,
		)
	}

	if closeErr != nil {
		return nil, fmt.Errorf(
			"close object %q response body: %w",
			key,
			closeErr,
		)
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
